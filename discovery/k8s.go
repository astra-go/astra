//go:build k8s || alltags

// Package discovery provides service registry implementations.
// This file contains the Kubernetes backend.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// annotationKey is the Pod annotation used to store Astra instance metadata.
const k8sAnnotationKey = "astra.io/instance"

// K8sConfig configures the Kubernetes registry.
//
// Kubernetes manages Pod lifecycle and network identity through Service and
// Endpoints objects. This registry reads Endpoints objects to discover
// healthy service instances, and watches them for changes using the
// Kubernetes Informer mechanism (list-and-watch).
//
// Register and Deregister write to a custom Pod Annotation
// ("astra.io/instances") rather than creating Service/Endpoint objects —
// that responsibility stays with the deployment platform.
type K8sConfig struct {
	// Namespace to query for Endpoints. Default: "default".
	Namespace string

	// InCluster reads credentials from the ServiceAccount mounted in the Pod.
	// Set to true when the application runs inside a Kubernetes cluster.
	InCluster bool

	// KubeconfigPath is the path to a kubeconfig file for out-of-cluster use.
	// When empty, ~/.kube/config is used.
	KubeconfigPath string
}

func (c *K8sConfig) setDefaults() {
	if c.Namespace == "" {
		c.Namespace = "default"
	}
}

// K8sRegistry implements Registry using the Kubernetes Endpoints API.
//
// # In-cluster usage (recommended for production)
//
//	reg, err := discovery.NewK8sRegistry(discovery.K8sConfig{
//	    Namespace: "production",
//	    InCluster: true,
//	})
//	instances, err := reg.Discover(ctx, "user-svc")
//
// # Out-of-cluster usage (local development)
//
//	reg, err := discovery.NewK8sRegistry(discovery.K8sConfig{
//	    Namespace:      "default",
//	    KubeconfigPath: os.Getenv("KUBECONFIG"),
//	})
type K8sRegistry struct {
	client    kubernetes.Interface
	namespace string

	mu       sync.RWMutex
	watchers map[string][]chan []*ServiceInstance
	cancelFn context.CancelFunc
	ctx      context.Context
}

// NewK8sRegistry creates a Kubernetes Registry.
func NewK8sRegistry(cfg K8sConfig) (*K8sRegistry, error) {
	cfg.setDefaults()

	var restCfg *rest.Config
	var err error

	if cfg.InCluster {
		restCfg, err = rest.InClusterConfig()
	} else {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		if cfg.KubeconfigPath != "" {
			rules.ExplicitPath = cfg.KubeconfigPath
		}
		restCfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			rules,
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s: build config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: new client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &K8sRegistry{
		client:    client,
		namespace: cfg.Namespace,
		watchers:  make(map[string][]chan []*ServiceInstance),
		cancelFn:  cancel,
		ctx:       ctx,
	}
	return r, nil
}

// Register writes the instance metadata as a Pod Annotation.
// The Pod name is used as the resource to annotate.
//
// Note: this is a lightweight annotation approach. In a typical Kubernetes
// deployment, services are discovered via Endpoints (see Discover). Use
// Register when you need Astra-specific metadata (scheme, weight) attached
// to the Pod.
func (r *K8sRegistry) Register(ctx context.Context, instance *ServiceInstance) error {
	if instance.ID == "" {
		return ErrInstanceIDEmpty
	}

	data, err := json.Marshal(k8sInstanceAnnotation{
		ID:       instance.ID,
		Name:     instance.Name,
		Address:  instance.Address,
		Scheme:   instance.Scheme,
		Weight:   instance.Weight,
		Metadata: instance.Metadata,
	})
	if err != nil {
		return fmt.Errorf("k8s: marshal instance: %w", err)
	}

	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				k8sAnnotationKey: string(data),
			},
		},
	}
	patchBytes, _ := json.Marshal(patch)

	_, err = r.client.CoreV1().Pods(r.namespace).
		Patch(ctx, instance.ID,
			"application/merge-patch+json",
			patchBytes,
			metav1.PatchOptions{},
		)
	if err != nil {
		return fmt.Errorf("k8s: register %s: %w", instance.ID, err)
	}
	return nil
}

// Deregister removes the Astra annotation from the Pod.
func (r *K8sRegistry) Deregister(ctx context.Context, instanceID string) error {
	if instanceID == "" {
		return ErrInstanceIDEmpty
	}
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				k8sAnnotationKey: nil,
			},
		},
	}
	patchBytes, _ := json.Marshal(patch)

	_, err := r.client.CoreV1().Pods(r.namespace).
		Patch(ctx, instanceID,
			"application/merge-patch+json",
			patchBytes,
			metav1.PatchOptions{},
		)
	if err != nil {
		return fmt.Errorf("k8s: deregister %s: %w", instanceID, err)
	}
	return nil
}

// Discover returns all ready Endpoint addresses for the given service name.
// The serviceName must match the Kubernetes Service name.
func (r *K8sRegistry) Discover(ctx context.Context, serviceName string) ([]*ServiceInstance, error) {
	ep, err := r.client.CoreV1().Endpoints(r.namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("k8s: get endpoints %s: %w", serviceName, err)
	}
	instances := endpointsToInstances(ep, serviceName)
	if len(instances) == 0 {
		return nil, ErrNoInstances
	}
	return instances, nil
}

// Watch returns a channel that receives updated instance lists whenever the
// Endpoints object for serviceName changes.
func (r *K8sRegistry) Watch(ctx context.Context, serviceName string) (<-chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 8)

	r.mu.Lock()
	r.watchers[serviceName] = append(r.watchers[serviceName], ch)
	r.mu.Unlock()

	go r.watchEndpoints(ctx, serviceName, ch)
	return ch, nil
}

// Close cancels all Watch goroutines.
func (r *K8sRegistry) Close() error {
	r.cancelFn()
	return nil
}

// watchEndpoints uses the k8s watch API to stream Endpoint changes.
func (r *K8sRegistry) watchEndpoints(ctx context.Context, serviceName string, ch chan []*ServiceInstance) {
	defer func() {
		r.mu.Lock()
		watchers := r.watchers[serviceName]
		for i, w := range watchers {
			if w == ch {
				r.watchers[serviceName] = append(watchers[:i], watchers[i+1:]...)
				break
			}
		}
		r.mu.Unlock()
		close(ch)
	}()

	watcher, err := r.client.CoreV1().Endpoints(r.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + serviceName,
	})
	if err != nil {
		return
	}
	defer watcher.Stop()

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			if event.Type == watch.Modified || event.Type == watch.Added {
				if ep, ok := event.Object.(*corev1.Endpoints); ok {
					instances := endpointsToInstances(ep, serviceName)
					select {
					case ch <- instances:
					default:
					}
				}
			}
		case <-ctx.Done():
			return
		case <-r.ctx.Done():
			return
		}
	}
}

// endpointsToInstances converts a k8s Endpoints object to discovery instances.
func endpointsToInstances(ep *corev1.Endpoints, serviceName string) []*ServiceInstance {
	var instances []*ServiceInstance
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			for _, port := range subset.Ports {
				id := addr.IP
				if addr.TargetRef != nil {
					id = addr.TargetRef.Name
				}
				instances = append(instances, &ServiceInstance{
					ID:      id,
					Name:    serviceName,
					Address: fmt.Sprintf("%s:%d", addr.IP, port.Port),
					Scheme:  "http",
					Weight:  1,
				})
			}
		}
	}
	return instances
}

// k8sInstanceAnnotation is the data stored in the astra.io/instance Pod annotation.
type k8sInstanceAnnotation struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Scheme   string            `json:"scheme"`
	Weight   int               `json:"weight"`
	Metadata map[string]string `json:"metadata,omitempty"`
}
