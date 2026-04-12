package principal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- example typed tools used across the adapter tests ---------------

type lookupArgs struct{ Query string }

type lookupTool struct{}

func (lookupTool) Name() string        { return "lookup_balance" }
func (lookupTool) Description() string { return "Look up a wallet balance (read-only)" }
func (lookupTool) Execute(
	ctx context.Context,
	principal PrincipalContext[CapReadOnly],
	args lookupArgs,
) (Result, error) {
	return Result{
		ForLLM:  fmt.Sprintf("balance for %s: 1.23 ETH", args.Query),
		ForUser: "1.23 ETH",
	}, nil
}

type writeFileArgs struct{ Path, Content string }

type writeFileTool struct{}

func (writeFileTool) Name() string        { return "write_memory_note" }
func (writeFileTool) Description() string { return "Write a note to workspace (writes local)" }
func (writeFileTool) Execute(
	ctx context.Context,
	p PrincipalContext[CapWritesLocal],
	args writeFileArgs,
) (Result, error) {
	return Result{ForLLM: fmt.Sprintf("wrote %s", args.Path)}, nil
}

type installSkillArgs struct{ SkillName string }

type installSkillTool struct{}

func (installSkillTool) Name() string        { return "install_skill" }
func (installSkillTool) Description() string { return "Install a skill into workspace (writes state)" }
func (installSkillTool) Execute(
	ctx context.Context,
	p PrincipalContext[CapWritesState],
	args installSkillArgs,
) (Result, error) {
	return Result{ForLLM: fmt.Sprintf("installed %s", args.SkillName)}, nil
}

type castArgs struct{ Message string }

type castTool struct{}

func (castTool) Name() string        { return "post_farcaster_cast" }
func (castTool) Description() string { return "Post to Farcaster (writes chain)" }
func (castTool) Execute(
	ctx context.Context,
	p PrincipalContext[CapWritesChain],
	args castArgs,
) (Result, error) {
	return Result{ForLLM: fmt.Sprintf("cast: %s", args.Message)}, nil
}

type stakeArgs struct{ AmountETH float64 }

type stakeTool struct{}

func (stakeTool) Name() string        { return "lido_stake" }
func (stakeTool) Description() string { return "Stake ETH to Lido (writes wallet)" }
func (stakeTool) Execute(
	ctx context.Context,
	principal PrincipalContext[CapWritesWallet],
	args stakeArgs,
) (Result, error) {
	if args.AmountETH <= 0 {
		return Result{}, errors.New("amount must be positive")
	}
	return Result{
		ForLLM:  fmt.Sprintf("staked %.4f ETH for %s", args.AmountETH, principal.AgentID()),
		ForUser: "Stake submitted",
	}, nil
}

// --- compile-time dispatch test ---------------------------------------

func TestTypedToolCompilesWithCorrectPrincipal(t *testing.T) {
	ctx := context.Background()
	wallet := mintPrincipal[CapWritesWallet](t, "writes_wallet")

	tool := stakeTool{}
	res, err := tool.Execute(ctx, wallet, stakeArgs{AmountETH: 0.5})
	if err != nil {
		t.Fatalf("stake tool err = %v, want nil", err)
	}
	if !strings.Contains(res.ForLLM, "staked 0.5000") {
		t.Fatalf("stake result = %q, want to contain staked 0.5000", res.ForLLM)
	}

	ro := NewReadOnly("agent-main", "user-alice", "acct-0x1", "cli")
	lookup := lookupTool{}
	res, err = lookup.Execute(ctx, ro, lookupArgs{Query: "0xdeadbeef"})
	if err != nil {
		t.Fatalf("lookup tool err = %v, want nil", err)
	}
	if !strings.Contains(res.ForLLM, "0xdeadbeef") {
		t.Fatalf("lookup result = %q, want to contain query string", res.ForLLM)
	}
}

// --- runtime adapter dispatch tests for every capability class --------

func TestAdaptersEnforceCapabilityAtRuntime(t *testing.T) {
	ctx := context.Background()

	lookupR := AdaptReadOnly(lookupTool{}, func(raw map[string]any) (lookupArgs, error) {
		q, _ := raw["query"].(string)
		return lookupArgs{Query: q}, nil
	})
	writeR := AdaptWritesLocal(writeFileTool{}, func(raw map[string]any) (writeFileArgs, error) {
		p, _ := raw["path"].(string)
		c, _ := raw["content"].(string)
		return writeFileArgs{Path: p, Content: c}, nil
	})
	installR := AdaptWritesState(installSkillTool{}, func(raw map[string]any) (installSkillArgs, error) {
		n, _ := raw["skill_name"].(string)
		return installSkillArgs{SkillName: n}, nil
	})
	castR := AdaptWritesChain(castTool{}, func(raw map[string]any) (castArgs, error) {
		m, _ := raw["message"].(string)
		return castArgs{Message: m}, nil
	})
	stakeR := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		amt, _ := raw["amount_eth"].(float64)
		return stakeArgs{AmountETH: amt}, nil
	})

	roUntyped := FromReadOnly(NewReadOnly("a1", "u1", "ac1", "ch1"))
	localUntyped := FromWritesLocal(mintPrincipal[CapWritesLocal](t, "writes_local"))
	stateUntyped := FromWritesState(mintPrincipal[CapWritesState](t, "writes_state"))
	chainUntyped := FromWritesChain(mintPrincipal[CapWritesChain](t, "writes_chain"))
	walletUntyped := FromWritesWallet(mintPrincipal[CapWritesWallet](t, "writes_wallet"))

	// Every tool rejects a read-only principal that tries to escalate.
	t.Run("ro→writes_local denied", func(t *testing.T) {
		_, err := writeR.RunUntyped(ctx, roUntyped, map[string]any{"path": "/tmp/x"})
		if !errors.Is(err, ErrInsufficientCap) {
			t.Fatalf("err = %v, want ErrInsufficientCap", err)
		}
	})
	t.Run("ro→writes_state denied", func(t *testing.T) {
		_, err := installR.RunUntyped(ctx, roUntyped, map[string]any{"skill_name": "x"})
		if !errors.Is(err, ErrInsufficientCap) {
			t.Fatalf("err = %v, want ErrInsufficientCap", err)
		}
	})
	t.Run("ro→writes_chain denied", func(t *testing.T) {
		_, err := castR.RunUntyped(ctx, roUntyped, map[string]any{"message": "gm"})
		if !errors.Is(err, ErrInsufficientCap) {
			t.Fatalf("err = %v, want ErrInsufficientCap", err)
		}
	})
	t.Run("ro→writes_wallet denied", func(t *testing.T) {
		_, err := stakeR.RunUntyped(ctx, roUntyped, map[string]any{"amount_eth": 0.5})
		if !errors.Is(err, ErrInsufficientCap) {
			t.Fatalf("err = %v, want ErrInsufficientCap", err)
		}
	})

	// Each matching principal succeeds on its own class.
	t.Run("writes_local principal runs writes_local tool", func(t *testing.T) {
		res, err := writeR.RunUntyped(ctx, localUntyped, map[string]any{"path": "/tmp/x"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(res.ForLLM, "wrote /tmp/x") {
			t.Fatalf("result = %q", res.ForLLM)
		}
	})
	t.Run("writes_state principal runs writes_state tool", func(t *testing.T) {
		res, err := installR.RunUntyped(ctx, stateUntyped, map[string]any{"skill_name": "crypto-wallet"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(res.ForLLM, "crypto-wallet") {
			t.Fatalf("result = %q", res.ForLLM)
		}
	})
	t.Run("writes_chain principal runs writes_chain tool", func(t *testing.T) {
		res, err := castR.RunUntyped(ctx, chainUntyped, map[string]any{"message": "gm"})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(res.ForLLM, "gm") {
			t.Fatalf("result = %q", res.ForLLM)
		}
	})
	t.Run("writes_wallet principal runs writes_wallet tool", func(t *testing.T) {
		res, err := stakeR.RunUntyped(ctx, walletUntyped, map[string]any{"amount_eth": 0.5})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if !strings.Contains(res.ForLLM, "staked 0.5000") {
			t.Fatalf("result = %q", res.ForLLM)
		}
	})

	// Monotonicity: a wallet principal can call every lower tool.
	t.Run("wallet runs read_only tool", func(t *testing.T) {
		_, err := lookupR.RunUntyped(ctx, walletUntyped, map[string]any{"query": "0xdef"})
		if err != nil {
			t.Fatalf("monotonic wallet → read_only should succeed: %v", err)
		}
	})
	t.Run("wallet runs writes_local tool", func(t *testing.T) {
		_, err := writeR.RunUntyped(ctx, walletUntyped, map[string]any{"path": "/tmp/y"})
		if err != nil {
			t.Fatalf("monotonic wallet → writes_local should succeed: %v", err)
		}
	})
	t.Run("wallet runs writes_state tool", func(t *testing.T) {
		_, err := installR.RunUntyped(ctx, walletUntyped, map[string]any{"skill_name": "defi-swap"})
		if err != nil {
			t.Fatalf("monotonic wallet → writes_state should succeed: %v", err)
		}
	})
	t.Run("wallet runs writes_chain tool", func(t *testing.T) {
		_, err := castR.RunUntyped(ctx, walletUntyped, map[string]any{"message": "hi"})
		if err != nil {
			t.Fatalf("monotonic wallet → writes_chain should succeed: %v", err)
		}
	})

	// Non-monotonicity: writes_local CANNOT escalate up the ladder.
	t.Run("writes_local cannot run writes_chain tool", func(t *testing.T) {
		_, err := castR.RunUntyped(ctx, localUntyped, map[string]any{"message": "hi"})
		if !errors.Is(err, ErrInsufficientCap) {
			t.Fatalf("err = %v, want ErrInsufficientCap", err)
		}
	})
}

// TestAdapterRejectsZeroPrincipal covers the fail-closed path when a
// buggy caller hands the adapter an UntypedPrincipal that was never
// constructed via one of the From* helpers.
func TestAdapterRejectsZeroPrincipal(t *testing.T) {
	ctx := context.Background()
	stakeR := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		return stakeArgs{}, nil
	})
	var zero UntypedPrincipal
	_, err := stakeR.RunUntyped(ctx, zero, map[string]any{"amount_eth": 0.5})
	if !errors.Is(err, ErrZeroPrincipal) {
		t.Fatalf("err = %v, want ErrZeroPrincipal", err)
	}
}

// TestAdaptersSurfaceMetadata verifies Name/Description/RequiredEffectClass
// bubble through the adapter.
func TestAdaptersSurfaceMetadata(t *testing.T) {
	stake := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		return stakeArgs{}, nil
	})
	if stake.Name() != "lido_stake" {
		t.Fatalf("Name = %q, want lido_stake", stake.Name())
	}
	if stake.RequiredEffectClass() != "writes_wallet" {
		t.Fatalf("RequiredEffectClass = %q, want writes_wallet", stake.RequiredEffectClass())
	}
	if !strings.Contains(stake.Description(), "writes wallet") {
		t.Fatalf("Description = %q, want to mention writes wallet", stake.Description())
	}

	lookup := AdaptReadOnly(lookupTool{}, func(raw map[string]any) (lookupArgs, error) {
		return lookupArgs{}, nil
	})
	if lookup.RequiredEffectClass() != "read_only" {
		t.Fatalf("RequiredEffectClass = %q, want read_only", lookup.RequiredEffectClass())
	}
}

// TestAdapterDecodeErrorSurfacesCleanly ensures a failing decoder
// returns a wrapped error with the tool name.
func TestAdapterDecodeErrorSurfacesCleanly(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("missing amount_eth")
	stakeR := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		return stakeArgs{}, sentinel
	})
	wallet := FromWritesWallet(mintPrincipal[CapWritesWallet](t, "writes_wallet"))
	_, err := stakeR.RunUntyped(ctx, wallet, map[string]any{})
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err should wrap sentinel, got: %v", err)
	}
	if !strings.Contains(err.Error(), "lido_stake") {
		t.Fatalf("err should name the tool, got: %v", err)
	}
}

// TestCapabilityLadderMonotonicity asserts the ladder holds across
// every rank combination. For each principal class, check every
// lower class is held and every higher class is not.
func TestCapabilityLadderMonotonicity(t *testing.T) {
	table := []struct {
		name       string
		untyped    UntypedPrincipal
		expectHeld []string
		expectMiss []string
	}{
		{
			name:       "read_only",
			untyped:    FromReadOnly(NewReadOnly("a", "u", "ac", "ch")),
			expectHeld: []string{"read_only"},
			expectMiss: []string{"writes_local", "writes_state", "writes_chain", "writes_wallet"},
		},
		{
			name:       "writes_local",
			untyped:    FromWritesLocal(mintPrincipal[CapWritesLocal](t, "writes_local")),
			expectHeld: []string{"read_only", "writes_local"},
			expectMiss: []string{"writes_state", "writes_chain", "writes_wallet"},
		},
		{
			name:       "writes_state",
			untyped:    FromWritesState(mintPrincipal[CapWritesState](t, "writes_state")),
			expectHeld: []string{"read_only", "writes_local", "writes_state"},
			expectMiss: []string{"writes_chain", "writes_wallet"},
		},
		{
			name:       "writes_chain",
			untyped:    FromWritesChain(mintPrincipal[CapWritesChain](t, "writes_chain")),
			expectHeld: []string{"read_only", "writes_local", "writes_state", "writes_chain"},
			expectMiss: []string{"writes_wallet"},
		},
		{
			name:       "writes_wallet",
			untyped:    FromWritesWallet(mintPrincipal[CapWritesWallet](t, "writes_wallet")),
			expectHeld: []string{"read_only", "writes_local", "writes_state", "writes_chain", "writes_wallet"},
			expectMiss: nil,
		},
	}

	for _, tc := range table {
		t.Run(tc.name, func(t *testing.T) {
			for _, cap := range tc.expectHeld {
				if !tc.untyped.HasCap(cap) {
					t.Errorf("%s principal should hold %s", tc.name, cap)
				}
			}
			for _, cap := range tc.expectMiss {
				if tc.untyped.HasCap(cap) {
					t.Errorf("%s principal should NOT hold %s", tc.name, cap)
				}
			}
		})
	}
}

// TestHasCapUnknownReturnsFalse covers fail-closed behavior when
// something unexpected reaches HasCap.
func TestHasCapUnknownReturnsFalse(t *testing.T) {
	wallet := FromWritesWallet(mintPrincipal[CapWritesWallet](t, "writes_wallet"))
	if wallet.HasCap("writes_world") {
		t.Fatal("unknown cap should return false")
	}
	if wallet.HasCap("") {
		t.Fatal("empty cap should return false")
	}
}

// TestZeroValuedPrincipalCannotPassAnyAdapter verifies that a zero-valued
// UntypedPrincipal is rejected by every adapter, not just writes_wallet.
// This covers the guardDispatch zero-check for all 5 capability classes.
func TestZeroValuedPrincipalCannotPassAnyAdapter(t *testing.T) {
	ctx := context.Background()
	var zero UntypedPrincipal

	adapters := []struct {
		name    string
		adapter Runnable
	}{
		{"read_only", AdaptReadOnly(lookupTool{}, func(raw map[string]any) (lookupArgs, error) {
			return lookupArgs{}, nil
		})},
		{"writes_local", AdaptWritesLocal(writeFileTool{}, func(raw map[string]any) (writeFileArgs, error) {
			return writeFileArgs{}, nil
		})},
		{"writes_state", AdaptWritesState(installSkillTool{}, func(raw map[string]any) (installSkillArgs, error) {
			return installSkillArgs{}, nil
		})},
		{"writes_chain", AdaptWritesChain(castTool{}, func(raw map[string]any) (castArgs, error) {
			return castArgs{}, nil
		})},
		{"writes_wallet", AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
			return stakeArgs{}, nil
		})},
	}

	for _, tc := range adapters {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.adapter.RunUntyped(ctx, zero, map[string]any{})
			if !errors.Is(err, ErrZeroPrincipal) {
				t.Errorf("zero principal should be rejected by %s adapter: got %v, want ErrZeroPrincipal", tc.name, err)
			}
		})
	}
}

// TestForgedWalletPrincipalCannotEscalate verifies that even if a caller
// manages to construct an UntypedPrincipal with a populated agent ID,
// it still cannot dispatch a writes_wallet tool unless it holds the
// writes_wallet capability. This is the runtime defense-in-depth layer
// that backs up the compile-time type system.
func TestForgedWalletPrincipalCannotEscalate(t *testing.T) {
	ctx := context.Background()

	stakeR := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		amt, _ := raw["amount_eth"].(float64)
		return stakeArgs{AmountETH: amt}, nil
	})

	// A read-only principal with populated identity fields
	roUntyped := FromReadOnly(NewReadOnly("agent-main", "user-alice", "acct-0x1", "cli"))
	if roUntyped.IsZero() {
		t.Fatal("read-only principal should not be zero")
	}
	if roUntyped.AgentID() != "agent-main" {
		t.Fatal("read-only principal should have populated agent ID")
	}

	// Despite having a populated identity, a read-only principal
	// must be blocked from dispatching a writes_wallet tool.
	_, err := stakeR.RunUntyped(ctx, roUntyped, map[string]any{"amount_eth": 1.0})
	if !errors.Is(err, ErrInsufficientCap) {
		t.Fatalf("read-only principal with populated identity should be rejected: got %v, want ErrInsufficientCap", err)
	}

	// A writes_local principal also cannot escalate to writes_wallet.
	localUntyped := FromWritesLocal(mintPrincipal[CapWritesLocal](t, "writes_local"))
	_, err = stakeR.RunUntyped(ctx, localUntyped, map[string]any{"amount_eth": 1.0})
	if !errors.Is(err, ErrInsufficientCap) {
		t.Fatalf("writes_local principal should not escalate to writes_wallet: got %v, want ErrInsufficientCap", err)
	}

	// A writes_chain principal also cannot escalate to writes_wallet
	// (one step below on the ladder).
	chainUntyped := FromWritesChain(mintPrincipal[CapWritesChain](t, "writes_chain"))
	_, err = stakeR.RunUntyped(ctx, chainUntyped, map[string]any{"amount_eth": 1.0})
	if !errors.Is(err, ErrInsufficientCap) {
		t.Fatalf("writes_chain principal should not escalate to writes_wallet: got %v, want ErrInsufficientCap", err)
	}

	// Only a writes_wallet principal should succeed.
	walletUntyped := FromWritesWallet(mintPrincipal[CapWritesWallet](t, "writes_wallet"))
	res, err := stakeR.RunUntyped(ctx, walletUntyped, map[string]any{"amount_eth": 0.5})
	if err != nil {
		t.Fatalf("wallet principal should succeed: %v", err)
	}
	if !strings.Contains(res.ForLLM, "staked 0.5000") {
		t.Errorf("result = %q, want to contain staked amount", res.ForLLM)
	}
}

// TestGuardDispatchErrorMessages verifies that the error messages from
// guardDispatch include the tool name and required capability, so
// operators can diagnose authorization failures from logs alone.
func TestGuardDispatchErrorMessages(t *testing.T) {
	ctx := context.Background()

	stakeR := AdaptWritesWallet(stakeTool{}, func(raw map[string]any) (stakeArgs, error) {
		return stakeArgs{}, nil
	})

	// Zero principal error should name the tool
	var zero UntypedPrincipal
	_, err := stakeR.RunUntyped(ctx, zero, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lido_stake") {
		t.Errorf("zero-principal error should name the tool: %v", err)
	}

	// Insufficient cap error should name the tool and required cap
	ro := FromReadOnly(NewReadOnly("a", "u", "ac", "ch"))
	_, err = stakeR.RunUntyped(ctx, ro, map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "lido_stake") {
		t.Errorf("insufficient-cap error should name the tool: %v", err)
	}
	if !strings.Contains(err.Error(), "writes_wallet") {
		t.Errorf("insufficient-cap error should name the required cap: %v", err)
	}
}

// mintPrincipal is a generic test helper that runs a read-only
// principal through the gate of the requested effect class.
func mintPrincipal[C Capability](t *testing.T, effect string) PrincipalContext[C] {
	t.Helper()
	ctx := context.Background()
	ro := NewReadOnly("agent-main", "user-alice", "acct-0x1", "cli")
	consent := SignedConsent{
		PrincipalLabel: Label(ro),
		EffectClass:    effect,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
		Signature:      []byte("test-sig"),
	}
	switch effect {
	case "writes_local":
		p, err := UpgradeToWritesLocal(ctx, ro, consent, &stubVerifier{})
		if err != nil {
			t.Fatalf("mint writes_local: %v", err)
		}
		return any(p).(PrincipalContext[C])
	case "writes_state":
		p, err := UpgradeToWritesState(ctx, ro, consent, &stubVerifier{})
		if err != nil {
			t.Fatalf("mint writes_state: %v", err)
		}
		return any(p).(PrincipalContext[C])
	case "writes_chain":
		p, err := UpgradeToWritesChain(ctx, ro, consent, &stubVerifier{})
		if err != nil {
			t.Fatalf("mint writes_chain: %v", err)
		}
		return any(p).(PrincipalContext[C])
	case "writes_wallet":
		p, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
		if err != nil {
			t.Fatalf("mint writes_wallet: %v", err)
		}
		return any(p).(PrincipalContext[C])
	default:
		t.Fatalf("unknown effect %q", effect)
	}
	var zero PrincipalContext[C]
	return zero
}
