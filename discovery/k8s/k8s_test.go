package k8s_test

import (
	"testing"

	"github.com/astra-go/astra/discovery"
	"github.com/astra-go/astra/discovery/k8s"
)

// ─── Compile-time interface check ────────────────────────────────────────────

var _ discovery.Registry = (*k8s.Registry)(nil)

// ─── New — error paths (no cluster required) ─────────────────────────────────

func TestNew_InCluster_OutsideCluster_ReturnsError(t *testing.T) {
	// rest.InClusterConfig() fails with ErrNotInCluster when the service-account
	// token file does not exist — which is always the case outside a Pod.
	_, err := k8s.New(k8s.Config{InCluster: true})
	if err == nil {
		t.Skip("running inside a Kubernetes cluster — InCluster test skipped")
	}
}

func TestNew_NonexistentKubeconfigPath_ReturnsError(t *testing.T) {
	_, err := k8s.New(k8s.Config{
		KubeconfigPath: "/nonexistent/path/to/kubeconfig",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig path")
	}
}
