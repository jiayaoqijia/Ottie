package principal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestCapabilityDistinctness is the ground-truth test that the
// phantom-typed generics produce distinct Go types AND that sealed
// construction means a hostile literal build produces a zero-valued
// principal that cannot pass any downstream check.
func TestCapabilityDistinctness(t *testing.T) {
	ro := NewReadOnly("agent-main", "user-alice", "acct-0x1", "cli")
	if ro.AgentID() != "agent-main" {
		t.Fatalf("AgentID = %q, want agent-main", ro.AgentID())
	}
	if Label(ro) != "agent=agent-main;user=user-alice;account=acct-0x1;channel=cli" {
		t.Fatalf("Label = %q, want canonical form", Label(ro))
	}

	// Hostile literal construction of a wallet-typed principal
	// compiles (Go has no way to forbid it) but produces a
	// zero-valued principal with empty identity fields.
	var forged PrincipalContext[CapWritesWallet]
	if !forged.IsZero() {
		t.Fatal("zero-valued literal wallet principal should report IsZero")
	}
	if forged.AgentID() != "" || Label(forged) != "agent=;user=;account=;channel=" {
		t.Fatal("zero-valued literal wallet principal should have empty identity")
	}

	// Proper upgrade through the gate with consent.
	ctx := context.Background()
	verifier := &stubVerifier{verifyResult: nil}
	consent := SignedConsent{
		PrincipalLabel: Label(ro),
		EffectClass:    "writes_wallet",
		ArgsHash:       "deadbeef",
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
		Signature:      []byte("sig"),
	}
	wallet, err := UpgradeToWritesWallet(ctx, ro, consent, verifier)
	if err != nil {
		t.Fatalf("UpgradeToWritesWallet err = %v, want nil", err)
	}
	if wallet.IsZero() {
		t.Fatal("upgraded wallet principal should have identity populated")
	}
	// Compile-time proof: requireWalletSigner only accepts
	// PrincipalContext[CapWritesWallet], and the fact this call
	// compiles is the proof that wallet has the right type parameter.
	requireWalletSigner(t, wallet)
}

// requireWalletSigner is a type-level assertion that compiles only
// if its argument has exactly type PrincipalContext[CapWritesWallet].
// The compile check is the test; the body just validates the
// runtime payload.
func requireWalletSigner(t *testing.T, p PrincipalContext[CapWritesWallet]) {
	t.Helper()
	if p.AgentID() == "" {
		t.Fatal("wallet principal AgentID should be populated after upgrade")
	}
}

// TestUpgradeGatesRejectMissingConsent covers the runtime failure
// modes of the upgrade path: no signature, expired consent, wrong
// label, wrong effect class, verifier error, nil verifier.
func TestUpgradeGatesRejectMissingConsent(t *testing.T) {
	ctx := context.Background()
	ro := NewReadOnly("agent-main", "user-alice", "acct-0x1", "cli")

	t.Run("no signature", func(t *testing.T) {
		_, err := UpgradeToWritesWallet(ctx, ro, SignedConsent{}, &stubVerifier{})
		if !errors.Is(err, ErrConsentRequired) {
			t.Fatalf("err = %v, want ErrConsentRequired", err)
		}
	})

	t.Run("expired consent", func(t *testing.T) {
		past := time.Now().Add(-time.Hour).Unix()
		consent := SignedConsent{
			PrincipalLabel: Label(ro),
			EffectClass:    "writes_wallet",
			ExpiresAt:      past,
			Signature:      []byte("sig"),
		}
		_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
		if !errors.Is(err, ErrConsentExpired) {
			t.Fatalf("err = %v, want ErrConsentExpired", err)
		}
	})

	t.Run("consent with zero expiry is treated as never-expires", func(t *testing.T) {
		consent := SignedConsent{
			PrincipalLabel: Label(ro),
			EffectClass:    "writes_wallet",
			ExpiresAt:      0,
			Signature:      []byte("sig"),
		}
		_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
		if err != nil {
			t.Fatalf("err = %v, want nil for zero-expiry consent", err)
		}
	})

	t.Run("wrong principal label", func(t *testing.T) {
		consent := SignedConsent{
			PrincipalLabel: "agent=other;user=bob;account=acct-0x2;channel=cli",
			EffectClass:    "writes_wallet",
			ExpiresAt:      time.Now().Add(time.Hour).Unix(),
			Signature:      []byte("sig"),
		}
		_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
		if !errors.Is(err, ErrConsentInvalid) {
			t.Fatalf("err = %v, want ErrConsentInvalid", err)
		}
		if !strings.Contains(err.Error(), "principal label") {
			t.Fatalf("err message should mention principal label, got: %v", err)
		}
	})

	t.Run("wrong effect class", func(t *testing.T) {
		consent := SignedConsent{
			PrincipalLabel: Label(ro),
			EffectClass:    "writes_local",
			ExpiresAt:      time.Now().Add(time.Hour).Unix(),
			Signature:      []byte("sig"),
		}
		_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
		if !errors.Is(err, ErrConsentInvalid) {
			t.Fatalf("err = %v, want ErrConsentInvalid", err)
		}
		if !strings.Contains(err.Error(), "effect class") {
			t.Fatalf("err message should mention effect class, got: %v", err)
		}
	})

	t.Run("verifier returns error", func(t *testing.T) {
		consent := SignedConsent{
			PrincipalLabel: Label(ro),
			EffectClass:    "writes_wallet",
			ExpiresAt:      time.Now().Add(time.Hour).Unix(),
			Signature:      []byte("sig"),
		}
		sentinel := errors.New("bad signature")
		_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{verifyResult: sentinel})
		if !errors.Is(err, ErrConsentInvalid) {
			t.Fatalf("err = %v, want ErrConsentInvalid", err)
		}
		if !errors.Is(err, sentinel) {
			t.Fatalf("err should wrap sentinel, got: %v", err)
		}
	})

	t.Run("nil verifier", func(t *testing.T) {
		consent := SignedConsent{
			PrincipalLabel: Label(ro),
			EffectClass:    "writes_wallet",
			ExpiresAt:      time.Now().Add(time.Hour).Unix(),
			Signature:      []byte("sig"),
		}
		_, err := UpgradeToWritesWallet(ctx, ro, consent, nil)
		if !errors.Is(err, ErrConsentInvalid) {
			t.Fatalf("err = %v, want ErrConsentInvalid", err)
		}
		if !strings.Contains(err.Error(), "verifier") {
			t.Fatalf("err message should mention verifier, got: %v", err)
		}
	})
}

// TestUpgradeGatesCoverAllFourCapabilities exercises the
// writes-local, writes-state, writes-chain, and writes-wallet paths.
// Every gate should succeed given a matching consent and fail given
// a mismatch.
func TestUpgradeGatesCoverAllFourCapabilities(t *testing.T) {
	ctx := context.Background()
	verifier := &stubVerifier{}

	cases := []struct {
		name    string
		effect  string
		upgrade func(
			context.Context,
			PrincipalContext[CapReadOnly],
			SignedConsent,
			ConsentVerifier,
		) error
	}{
		{
			name:   "writes_local",
			effect: "writes_local",
			upgrade: func(ctx context.Context, p PrincipalContext[CapReadOnly], c SignedConsent, v ConsentVerifier) error {
				_, err := UpgradeToWritesLocal(ctx, p, c, v)
				return err
			},
		},
		{
			name:   "writes_state",
			effect: "writes_state",
			upgrade: func(ctx context.Context, p PrincipalContext[CapReadOnly], c SignedConsent, v ConsentVerifier) error {
				_, err := UpgradeToWritesState(ctx, p, c, v)
				return err
			},
		},
		{
			name:   "writes_chain",
			effect: "writes_chain",
			upgrade: func(ctx context.Context, p PrincipalContext[CapReadOnly], c SignedConsent, v ConsentVerifier) error {
				_, err := UpgradeToWritesChain(ctx, p, c, v)
				return err
			},
		},
		{
			name:   "writes_wallet",
			effect: "writes_wallet",
			upgrade: func(ctx context.Context, p PrincipalContext[CapReadOnly], c SignedConsent, v ConsentVerifier) error {
				_, err := UpgradeToWritesWallet(ctx, p, c, v)
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ro := NewReadOnly("a1", "u1", "ac1", "ch1")
			consent := SignedConsent{
				PrincipalLabel: Label(ro),
				EffectClass:    tc.effect,
				ExpiresAt:      time.Now().Add(time.Hour).Unix(),
				Signature:      []byte("sig"),
			}
			if err := tc.upgrade(ctx, ro, consent, verifier); err != nil {
				t.Fatalf("upgrade to %s failed: %v", tc.effect, err)
			}

			wrong := consent
			if tc.effect == "writes_wallet" {
				wrong.EffectClass = "writes_local"
			} else {
				wrong.EffectClass = "writes_wallet"
			}
			err := tc.upgrade(ctx, ro, wrong, verifier)
			if !errors.Is(err, ErrConsentInvalid) {
				t.Fatalf("expected ErrConsentInvalid for mismatched effect, got: %v", err)
			}
		})
	}
}

// TestExpiryBoundaryAtExactNow verifies the unix-seconds boundary: a
// consent whose ExpiresAt is exactly "now" is treated as expired
// (not still-valid), because the gate uses `>=` comparison to avoid
// a one-second replay window.
func TestExpiryBoundaryAtExactNow(t *testing.T) {
	ctx := context.Background()
	ro := NewReadOnly("a", "u", "ac", "ch")

	fixedNow := time.Unix(1700000000, 0)
	original := nowFn
	nowFn = func() time.Time { return fixedNow }
	defer func() { nowFn = original }()

	consent := SignedConsent{
		PrincipalLabel: Label(ro),
		EffectClass:    "writes_wallet",
		ExpiresAt:      fixedNow.Unix(), // exactly now
		Signature:      []byte("sig"),
	}
	_, err := UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
	if !errors.Is(err, ErrConsentExpired) {
		t.Fatalf("consent at exact now should be expired, got: %v", err)
	}

	consent.ExpiresAt = fixedNow.Unix() + 1
	_, err = UpgradeToWritesWallet(ctx, ro, consent, &stubVerifier{})
	if err != nil {
		t.Fatalf("consent one second in the future should succeed, got: %v", err)
	}
}

// TestSerializePrincipalLabelWithSpecialCharacters verifies that
// principal labels with ; or = in field values produce labels that
// can be distinguished from labels with different field assignments.
// This documents the current unescaped behavior as a known limitation.
func TestSerializePrincipalLabelWithSpecialCharacters(t *testing.T) {
	// A user scope containing ";" creates an ambiguous label.
	// The legitimate principal:
	legit := NewReadOnly("agent-main", "alice", "acct-0x1", "cli")
	legitLabel := Label(legit)

	// An attacker who controls userScope tries to impersonate:
	attacker := NewReadOnly("agent-main", "alice;account=acct-0x1;channel=evil", "", "")
	attackerLabel := Label(attacker)

	// These labels MUST be different even though a naive parser
	// might extract the same agent= and user= prefix from both.
	if legitLabel == attackerLabel {
		t.Fatalf("labels must differ:\n  legit:    %q\n  attacker: %q", legitLabel, attackerLabel)
	}

	// Verify the label format contains the raw special characters
	// (documenting current behavior — no escaping).
	if !strings.Contains(attackerLabel, ";account=acct-0x1;channel=evil") {
		t.Errorf("attacker label should contain injected fields: %q", attackerLabel)
	}

	// An "=" in the agent ID should not split the field.
	eqAgent := NewReadOnly("agent=evil", "user", "acct", "ch")
	eqLabel := Label(eqAgent)
	if !strings.Contains(eqLabel, "agent=agent=evil;") {
		t.Errorf("agent with = should appear literally: %q", eqLabel)
	}
}

type stubVerifier struct {
	verifyResult error
	seen         SignedConsent
}

func (v *stubVerifier) Verify(ctx context.Context, c SignedConsent) error {
	v.seen = c
	return v.verifyResult
}
