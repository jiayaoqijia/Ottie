package agent

import (
	"testing"
	"time"
)

func TestResolveAgentTimeout(t *testing.T) {
	tests := []struct {
		name string
		opts TimeoutOptions
		want time.Duration
	}{
		{
			name: "override_ms takes highest priority",
			opts: TimeoutOptions{OverrideMs: 5000, OverrideSec: 10, ConfigSec: 20, DefaultSec: 30},
			want: 5 * time.Second,
		},
		{
			name: "override_ms sub-second clamped to 1s",
			opts: TimeoutOptions{OverrideMs: 500},
			want: 1 * time.Second,
		},
		{
			name: "override_sec used when override_ms is zero",
			opts: TimeoutOptions{OverrideMs: 0, OverrideSec: 15, ConfigSec: 20},
			want: 15 * time.Second,
		},
		{
			name: "negative override_ms falls through to override_sec",
			opts: TimeoutOptions{OverrideMs: -1, OverrideSec: 10},
			want: 10 * time.Second,
		},
		{
			name: "config_sec used when overrides are zero",
			opts: TimeoutOptions{OverrideMs: 0, OverrideSec: 0, ConfigSec: 30},
			want: 30 * time.Second,
		},
		{
			name: "negative override_sec falls through to config_sec",
			opts: TimeoutOptions{OverrideSec: -1, ConfigSec: 25},
			want: 25 * time.Second,
		},
		{
			name: "default_sec used when all above are zero",
			opts: TimeoutOptions{DefaultSec: 120},
			want: 120 * time.Second,
		},
		{
			name: "default_sec zero uses 600",
			opts: TimeoutOptions{},
			want: 600 * time.Second,
		},
		{
			name: "negative config_sec falls through to default",
			opts: TimeoutOptions{ConfigSec: -1, DefaultSec: 90},
			want: 90 * time.Second,
		},
		{
			name: "all negative falls through to default 600",
			opts: TimeoutOptions{OverrideMs: -1, OverrideSec: -1, ConfigSec: -1, DefaultSec: 0},
			want: 600 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAgentTimeout(tt.opts)
			if got != tt.want {
				t.Errorf("ResolveAgentTimeout(%+v) = %v, want %v", tt.opts, got, tt.want)
			}
		})
	}
}
