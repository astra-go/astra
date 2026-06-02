//go:build k8s || alltags

package discovery_test

import (
	"testing"

	"github.com/astra-go/astra/discovery"
)

// ─── Compile-time interface check ────────────────────────────────────────────

var _ discovery.Registry = (*discovery.K8sRegistry)(nil)

// ─── New — error paths (no cluster required) ─────────────────────────────────

func TestNewK8sRegistry_InCluster_OutsideCluster_ReturnsError(t *testing.T) {
	// rest.InClusterConfig() fails with ErrNotInCluster when the service-account
	// token file does not exist — which is always the case outside a Pod.
	_, err := discovery.NewK8sRegistry(discovery.K8sConfig{InCluster: true})
	if err == nil {
		t.Skip("running inside a Kubernetes cluster — InCluster test skipped")
	}
}

func TestNewK8sRegistry_NonexistentKubeconfigPath_ReturnsError(t *testing.T) {
	_, err := discovery.NewK8sRegistry(discovery.K8sConfig{
		KubeconfigPath: "/nonexistent/path/to/kubeconfig",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig path")
	}
}
