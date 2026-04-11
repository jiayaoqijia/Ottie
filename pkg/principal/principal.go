// Package principal implements typed principal contexts and capability-
// bearing tool dispatch for Ottie. It is the load-bearing implementation
// of R6 leapfrog #2 / R7 §4.3: making unauthorized wallet writes a
// COMPILE-TIME type error and a runtime type error, not a casual runtime
// check.
//
// The shape is phantom-typed generics plus sealed construction:
//
//  1. Each capability class is a distinct zero-size empty struct; Go
//     1.18+ generic type-parameter distinctness makes
//     PrincipalContext[CapReadOnly] and PrincipalContext[CapWritesWallet]
//     different types at the type-checker level. A direct call
//     lidoStake.Execute(ctx, readOnly, args) fails `go build`.
//
//  2. The PrincipalContext struct is opaque from outside the package:
//     all four identity fields are unexported and can only be set by
//     the package's own NewReadOnly and UpgradeTo* gate functions.
//     Literal construction like PrincipalContext[CapWritesWallet]{}
//     still compiles, but produces a zero-valued principal that no
//     useful code will accept (the runtime adapter requires a
//     populated identity label and the action ledger rejects empty
//     principal labels). A hostile caller cannot construct a
//     meaningfully-populated high-capability principal without passing
//     through an upgrade gate.
//
//  3. Each upgrade gate validates SignedConsent at runtime: signature
//     non-empty, principal label matches, effect class matches,
//     expiry in the future, verifier approves. All five failure
//     modes are returned as wrapped errors so the caller can
//     disambiguate.
//
// Layering:
//
//   - Engineers writing new tools implement TypedTool[C, T] directly
//     and get compile-time safety via Go's type system.
//   - The runtime registry (pkg/tools) holds Runnable adapter values
//     that carry closed-over type information so they can decode raw
//     JSON args into T and check the runtime principal's capability
//     against the tool's declared class. Defense in depth: engineer
//     call sites are compile-time safe, model-dispatch call sites are
//     runtime safe.
//
// Capability upgrades are explicit and time-bounded. The only way to
// turn a read-only principal into one that can sign a transaction is
// to call the UpgradeTo* gate, which takes a SignedConsent with a
// principal label, effect class, args hash, expiry, and signature.
// A code path that bypasses the gate cannot produce a populated
// PrincipalContext[CapWritesWallet] value.
package principal

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Capability is the closed type union over capability markers. Each
// marker is a distinct empty struct so that PrincipalContext[CapReadOnly]
// and PrincipalContext[CapWritesWallet] are different types at the
// type-checker level.
//
// The union form is a Go 1.18+ generic type constraint. A concrete
// instantiation must be exactly one of the listed markers; any other
// type is rejected by the compiler.
type Capability interface {
	CapReadOnly | CapWritesLocal | CapWritesState | CapWritesChain | CapWritesWallet
}

// CapReadOnly marks a principal that may only read public state —
// query balances, fetch quotes, read memory. It cannot write to
// workspace, filesystem, agent state, on-chain state, or wallets.
type CapReadOnly struct{}

// CapWritesLocal marks a principal that may write workspace or
// filesystem state (files, MEMORY.md, session state) but not chain
// state or wallets.
type CapWritesLocal struct{}

// CapWritesState marks a principal that may modify agent state:
// install skills, change routing, update configuration. Narrower than
// CapWritesLocal because it covers internal state changes that could
// affect future agent behavior.
type CapWritesState struct{}

// CapWritesChain marks a principal that may submit state-changing
// operations to off-chain services that affect on-chain outcomes
// (e.g., submit a swap quote, post a Farcaster cast, call a DeFi API
// that routes through a protocol). Narrower than CapWritesWallet
// because it does not control private keys.
type CapWritesChain struct{}

// CapWritesWallet marks a principal that may sign transactions with
// the user's private key and broadcast them to the chain. This is
// the highest-privilege capability and the one that R6 leapfrog #2
// was designed to protect. A principal with this capability can
// drain funds; getting here requires passing an explicit upgrade
// gate with SignedConsent.
type CapWritesWallet struct{}

// PrincipalContext carries the identity and authority of the party
// making a tool call. The C type parameter encodes the capability
// class at the type level; two principals with different capability
// markers are distinct Go types and the compiler refuses to assign
// one to the other.
//
// All four identity fields are unexported. The only construction
// paths are:
//
//   - NewReadOnly (for initial session/tool-call entry)
//   - UpgradeToWritesLocal / UpgradeToWritesState / UpgradeToWritesChain
//     / UpgradeToWritesWallet (consent-gated upgrades)
//   - The internal runtime adapter path (reconstructed from
//     UntypedPrincipal during model-driven dispatch)
//
// Literal construction like PrincipalContext[CapWritesWallet]{} still
// compiles but produces a zero-valued principal with empty identity
// fields, which cannot pass any downstream check.
type PrincipalContext[C Capability] struct {
	agentID      string
	userScope    string
	accountScope string
	channelScope string
}

// AgentID returns the agent identifier the principal is acting as.
func (p PrincipalContext[C]) AgentID() string { return p.agentID }

// UserScope returns the per-user scope label.
func (p PrincipalContext[C]) UserScope() string { return p.userScope }

// AccountScope returns the per-crypto-account scope label. May be
// empty for principals not bound to a specific account.
func (p PrincipalContext[C]) AccountScope() string { return p.accountScope }

// ChannelScope returns the per-channel scope label. Non-empty for
// principals acting on behalf of a specific messaging channel.
func (p PrincipalContext[C]) ChannelScope() string { return p.channelScope }

// IsZero reports whether the principal has no identity fields set. A
// zero-valued principal cannot be used for any real operation; the
// runtime adapter refuses dispatch on a zero principal.
func (p PrincipalContext[C]) IsZero() bool {
	return p.agentID == "" && p.userScope == "" && p.accountScope == "" && p.channelScope == ""
}

// NewReadOnly constructs a read-only principal. This is the default
// starting point for any tool invocation; upgrades go through the
// gate functions below.
func NewReadOnly(agentID, userScope, accountScope, channelScope string) PrincipalContext[CapReadOnly] {
	return PrincipalContext[CapReadOnly]{
		agentID:      agentID,
		userScope:    userScope,
		accountScope: accountScope,
		channelScope: channelScope,
	}
}

// SignedConsent is the token a user passes when approving a
// higher-privilege operation. It is produced by the consent UI
// (CLI prompt, web dialog, Telegram approval message) and carries a
// hash over the specific operation being authorized. The gate below
// checks everything: signature presence, principal label,
// effect class, expiry, and verifier approval. ArgsHash is passed
// through to the verifier so it can check that the signed bytes
// cover the exact tool arguments the upgrade is being consumed for.
type SignedConsent struct {
	// PrincipalLabel is the serialized (agent_id, user_scope,
	// account_scope, channel_scope) at the moment the consent was
	// produced. It must match the principal being upgraded.
	PrincipalLabel string

	// EffectClass is the risk class being authorized: "writes_local",
	// "writes_state", "writes_chain", "writes_wallet".
	EffectClass string

	// ArgsHash is the sha256 over the canonicalized tool args, so an
	// attacker cannot swap out the arguments after consent. The gate
	// below passes this through to the verifier; the verifier is the
	// component that signs over the hash and therefore the component
	// that can check it.
	ArgsHash string

	// ExpiresAt is a unix timestamp; the upgrade gate rejects
	// expired consents. Zero is treated as "never expires" but
	// production code should always set a non-zero value.
	ExpiresAt int64

	// Signature is the user's cryptographic signature over the
	// above fields. The exact signing scheme (ed25519, EIP-712, etc.)
	// is decided at the channel layer.
	Signature []byte
}

// ConsentVerifier validates SignedConsent tokens. The pkg/principal
// package does not care which scheme produces the signature — the
// channel implementing user-facing consent provides a verifier.
type ConsentVerifier interface {
	Verify(ctx context.Context, c SignedConsent) error
}

// ErrConsentRequired is returned by upgrade gates when no consent
// was provided.
var ErrConsentRequired = errors.New("principal: consent required for capability upgrade")

// ErrConsentInvalid is returned when the supplied consent does not
// match the requested upgrade (wrong label, wrong effect, verifier
// rejection, nil verifier).
var ErrConsentInvalid = errors.New("principal: consent invalid for requested upgrade")

// ErrConsentExpired is returned when the consent's ExpiresAt is in
// the past at the moment of upgrade.
var ErrConsentExpired = errors.New("principal: consent expired")

// nowFn is swappable for tests. Default to the real clock.
var nowFn = func() time.Time { return time.Now() }

// UpgradeToWritesLocal is the gate for CapReadOnly → CapWritesLocal.
// Typical use: a tool that wants to write to workspace must pass
// through this gate first. The gate is runtime-checked because the
// consent validation is runtime behavior; the returned principal is
// compile-time typed so downstream tool calls are statically
// guarded.
func UpgradeToWritesLocal(
	ctx context.Context,
	p PrincipalContext[CapReadOnly],
	consent SignedConsent,
	verifier ConsentVerifier,
) (PrincipalContext[CapWritesLocal], error) {
	if err := validateUpgrade(ctx, p, consent, verifier, "writes_local"); err != nil {
		return PrincipalContext[CapWritesLocal]{}, err
	}
	return PrincipalContext[CapWritesLocal]{
		agentID:      p.agentID,
		userScope:    p.userScope,
		accountScope: p.accountScope,
		channelScope: p.channelScope,
	}, nil
}

// UpgradeToWritesState is the gate for CapReadOnly → CapWritesState.
// Covers skill installation, routing changes, configuration updates.
func UpgradeToWritesState(
	ctx context.Context,
	p PrincipalContext[CapReadOnly],
	consent SignedConsent,
	verifier ConsentVerifier,
) (PrincipalContext[CapWritesState], error) {
	if err := validateUpgrade(ctx, p, consent, verifier, "writes_state"); err != nil {
		return PrincipalContext[CapWritesState]{}, err
	}
	return PrincipalContext[CapWritesState]{
		agentID:      p.agentID,
		userScope:    p.userScope,
		accountScope: p.accountScope,
		channelScope: p.channelScope,
	}, nil
}

// UpgradeToWritesChain is the gate for CapReadOnly → CapWritesChain.
// Covers state-changing off-chain operations that route through
// on-chain protocols (e.g., submit a swap quote, post a cast).
func UpgradeToWritesChain(
	ctx context.Context,
	p PrincipalContext[CapReadOnly],
	consent SignedConsent,
	verifier ConsentVerifier,
) (PrincipalContext[CapWritesChain], error) {
	if err := validateUpgrade(ctx, p, consent, verifier, "writes_chain"); err != nil {
		return PrincipalContext[CapWritesChain]{}, err
	}
	return PrincipalContext[CapWritesChain]{
		agentID:      p.agentID,
		userScope:    p.userScope,
		accountScope: p.accountScope,
		channelScope: p.channelScope,
	}, nil
}

// UpgradeToWritesWallet is the gate for CapReadOnly → CapWritesWallet.
// This is the highest-privilege upgrade: the resulting principal can
// sign transactions with the user's private key. The gate enforces
// that the consent is non-empty, unexpired, label-matched,
// effect-matched, and verifier-approved. A code path that bypasses
// this gate cannot construct a meaningfully-populated
// PrincipalContext[CapWritesWallet] value.
func UpgradeToWritesWallet(
	ctx context.Context,
	p PrincipalContext[CapReadOnly],
	consent SignedConsent,
	verifier ConsentVerifier,
) (PrincipalContext[CapWritesWallet], error) {
	if err := validateUpgrade(ctx, p, consent, verifier, "writes_wallet"); err != nil {
		return PrincipalContext[CapWritesWallet]{}, err
	}
	return PrincipalContext[CapWritesWallet]{
		agentID:      p.agentID,
		userScope:    p.userScope,
		accountScope: p.accountScope,
		channelScope: p.channelScope,
	}, nil
}

// validateUpgrade is the shared runtime path for every upgrade gate.
// It runs five checks in order: consent non-empty, not expired,
// principal label matches, effect class matches, verifier approves.
// Returning on the first failure keeps the error message precise.
func validateUpgrade(
	ctx context.Context,
	p PrincipalContext[CapReadOnly],
	consent SignedConsent,
	verifier ConsentVerifier,
	expectedEffect string,
) error {
	if len(consent.Signature) == 0 {
		return ErrConsentRequired
	}
	if consent.ExpiresAt != 0 && nowFn().Unix() >= consent.ExpiresAt {
		return fmt.Errorf("%w: expires_at %d <= now", ErrConsentExpired, consent.ExpiresAt)
	}
	label := serializePrincipalLabel(p.agentID, p.userScope, p.accountScope, p.channelScope)
	if consent.PrincipalLabel != label {
		return fmt.Errorf(
			"%w: consent principal label %q does not match upgrading principal %q",
			ErrConsentInvalid, consent.PrincipalLabel, label,
		)
	}
	if consent.EffectClass != expectedEffect {
		return fmt.Errorf(
			"%w: consent effect class %q does not match requested %q",
			ErrConsentInvalid, consent.EffectClass, expectedEffect,
		)
	}
	if verifier == nil {
		return fmt.Errorf("%w: no verifier supplied", ErrConsentInvalid)
	}
	if err := verifier.Verify(ctx, consent); err != nil {
		return fmt.Errorf("%w: %w", ErrConsentInvalid, err)
	}
	return nil
}

// serializePrincipalLabel produces a canonical string form of the
// principal label. It must be deterministic across machines so a
// consent produced on one node can be verified on another.
func serializePrincipalLabel(agentID, userScope, accountScope, channelScope string) string {
	return fmt.Sprintf("agent=%s;user=%s;account=%s;channel=%s",
		agentID, userScope, accountScope, channelScope)
}

// Label returns the canonical principal label string for any typed
// PrincipalContext. It is the form that gets written into SQLite
// rows for recall filtering and into the action ledger for audit.
func Label[C Capability](p PrincipalContext[C]) string {
	return serializePrincipalLabel(p.agentID, p.userScope, p.accountScope, p.channelScope)
}
