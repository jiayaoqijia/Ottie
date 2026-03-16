package agent

import "time"

// TimeoutOptions holds the priority hierarchy for resolving agent timeouts.
type TimeoutOptions struct {
	OverrideMs  int // highest priority
	OverrideSec int // second priority
	ConfigSec   int // from config
	DefaultSec  int // fallback, use 600 if 0
}

// ResolveAgentTimeout resolves timeout from a priority hierarchy.
// Zero means no timeout. Negative values fall through to next level.
// Result is clamped to minimum 1 second (unless zero = no timeout).
func ResolveAgentTimeout(opts TimeoutOptions) time.Duration {
	var seconds int
	var chosen bool

	// Priority 1: OverrideMs
	if opts.OverrideMs > 0 {
		d := time.Duration(opts.OverrideMs) * time.Millisecond
		if d < time.Second {
			d = time.Second
		}
		return d
	}
	if opts.OverrideMs == 0 {
		// zero means no timeout at this level — fall through
	} else if opts.OverrideMs < 0 {
		// negative — skip to next level
	}

	// Priority 2: OverrideSec
	if opts.OverrideSec > 0 {
		seconds = opts.OverrideSec
		chosen = true
	} else if opts.OverrideSec < 0 {
		// negative — skip to next level
	}

	// Priority 3: ConfigSec
	if !chosen {
		if opts.ConfigSec > 0 {
			seconds = opts.ConfigSec
			chosen = true
		} else if opts.ConfigSec < 0 {
			// negative — skip to next level
		}
	}

	// Priority 4: DefaultSec
	if !chosen {
		seconds = opts.DefaultSec
		if seconds == 0 {
			seconds = 600
		}
	}

	if seconds == 0 {
		return 0
	}
	if seconds < 1 {
		seconds = 1
	}

	return time.Duration(seconds) * time.Second
}
