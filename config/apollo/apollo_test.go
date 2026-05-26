package apollo_test

import (
	"testing"

	apollocfg "github.com/astra-go/astra/config/apollo"
)

// ─── Validation — no server required ─────────────────────────────────────────

func TestNew_MissingAppID_ReturnsError(t *testing.T) {
	_, err := apollocfg.New(apollocfg.Config{
		MetaAddr: "http://localhost:8080",
	})
	if err == nil {
		t.Fatal("expected error when AppID is empty")
	}
}

func TestNew_MissingMetaAddr_ReturnsError(t *testing.T) {
	_, err := apollocfg.New(apollocfg.Config{
		AppID: "my-app",
	})
	if err == nil {
		t.Fatal("expected error when MetaAddr is empty")
	}
}

// TestNew_BothMissing_ReturnsError verifies that having neither AppID nor
// MetaAddr returns an error (the first validation in New fires).
func TestNew_BothMissing_ReturnsError(t *testing.T) {
	_, err := apollocfg.New(apollocfg.Config{})
	if err == nil {
		t.Fatal("expected error when both AppID and MetaAddr are empty")
	}
}
