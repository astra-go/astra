package quic

import (
	"testing"
	"time"
)

func TestWithConnectionMigration(t *testing.T) {
	tests := []struct {
		name string
		mode ConnectionMigrationMode
		want ConnectionMigrationMode
	}{
		{
			name: "MigrationDisabled",
			mode: MigrationDisabled,
			want: MigrationDisabled,
		},
		{
			name: "MigrationNATRebindingOnly",
			mode: MigrationNATRebindingOnly,
			want: MigrationNATRebindingOnly,
		},
		{
			name: "MigrationFullyEnabled",
			mode: MigrationFullyEnabled,
			want: MigrationFullyEnabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := defaultQUICOptions()
			WithConnectionMigration(tt.mode)(opts)

			if opts.ConnectionMigration != tt.want {
				t.Errorf("WithConnectionMigration() = %v, want %v",
					opts.ConnectionMigration, tt.want)
			}
		})
	}
}

func TestDefaultConnectionMigration(t *testing.T) {
	opts := defaultQUICOptions()

	if opts.ConnectionMigration != MigrationFullyEnabled {
		t.Errorf("defaultQUICOptions().ConnectionMigration = %v, want MigrationFullyEnabled",
			opts.ConnectionMigration)
	}
}

func TestDefaultQUICOptionsComplete(t *testing.T) {
	opts := defaultQUICOptions()

	if opts.MaxIdleTimeout != 30*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 30s", opts.MaxIdleTimeout)
	}
	if opts.MaxIncomingStreams != 100 {
		t.Errorf("MaxIncomingStreams = %v, want 100", opts.MaxIncomingStreams)
	}
	if opts.AltSvcMaxAge != 86400 {
		t.Errorf("AltSvcMaxAge = %v, want 86400", opts.AltSvcMaxAge)
	}
	if opts.Mode != ServerModeDualStack {
		t.Errorf("Mode = %v, want ServerModeDualStack", opts.Mode)
	}
	if opts.ConnectionMigration != MigrationFullyEnabled {
		t.Errorf("ConnectionMigration = %v, want MigrationFullyEnabled",
			opts.ConnectionMigration)
	}
}
