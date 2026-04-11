// TypedTool is the compile-time-checked counterpart to the runtime
// pkg/tools.Tool interface. The two worlds co-exist by design:
//
//   - Engineers writing new tools implement TypedTool[C, T] and get
//     capability enforcement at compile time. The tool's Execute
//     method takes a PrincipalContext[C] parameter, so a caller
//     holding the wrong-capability principal fails at `go build`.
//
//   - The runtime registry (pkg/tools) holds adapter values that
//     know how to decode string JSON args into T and how to check
//     the principal's capability against the tool's declared
//     RequiredCap at runtime. This is the only path model-driven
//     dispatch can take — the model never has type information, so
//     the registry has to enforce the cap class at runtime on its
//     behalf.
//
// Defense in depth: engineers get compile-time safety, the model
// gets runtime safety. Neither layer can accidentally drop an
// unauthorized signing call through.
//
// UntypedPrincipal is constructed only by the From* helpers below,
// which in turn take typed PrincipalContext[C] values as input. The
// struct fields are unexported so hostile code cannot construct an
// UntypedPrincipal with hand-forged capabilities.

package principal

import (
	"context"
	"errors"
	"fmt"
)

// TypedTool is the typed interface a tool implementer writes against.
// C is the required capability class; T is the typed argument struct.
// Execute receives the principal and the typed args directly; a
// caller with the wrong C fails the compile.
type TypedTool[C Capability, T any] interface {
	Name() string
	Description() string
	Execute(ctx context.Context, principal PrincipalContext[C], args T) (Result, error)
}

// Result is a minimal tool result. The runtime tool registry has its
// own richer ToolResult type; this one is the typed-side equivalent
// and gets adapted at the registry boundary. Kept small on purpose
// so pkg/principal has no dependency on pkg/tools. Both structs must
// be kept in sync when fields are added or removed.
type Result struct {
	ForLLM  string
	ForUser string
	Silent  bool
	IsError bool
}

// UntypedPrincipal is the runtime-only view of a principal that the
// registry dispatches with. The registry does not know which C class
// the caller holds at compile time (it holds a map of runnables), so
// it passes this struct and lets the adapter check the cap set
// against the tool's RequiredEffectClass.
//
// The fields are unexported: the only way to construct an
// UntypedPrincipal is to pass a typed PrincipalContext[C] through
// one of the From* helpers below. This prevents a hostile caller
// from building an untyped principal with hand-forged capabilities.
type UntypedPrincipal struct {
	agentID      string
	userScope    string
	accountScope string
	channelScope string
	// highestCap is the monotonic-ladder position the principal
	// holds. It is a single string so it cannot be mutated into a
	// multi-cap bag, and it is unexported so only the From*
	// constructors can set it.
	highestCap string
}

// AgentID, UserScope, AccountScope, and ChannelScope mirror the
// typed principal's identity fields for runtime consumers that
// need to build a principal label or audit row.
func (u UntypedPrincipal) AgentID() string      { return u.agentID }
func (u UntypedPrincipal) UserScope() string    { return u.userScope }
func (u UntypedPrincipal) AccountScope() string { return u.accountScope }
func (u UntypedPrincipal) ChannelScope() string { return u.channelScope }

// HighestCap returns the monotonic-ladder position the principal
// holds (e.g., "writes_wallet"). Useful for audit logging.
func (u UntypedPrincipal) HighestCap() string { return u.highestCap }

// IsZero reports whether the principal has no identity fields set.
// The runtime adapter refuses dispatch on a zero principal.
func (u UntypedPrincipal) IsZero() bool {
	return u.agentID == "" && u.userScope == "" && u.accountScope == "" && u.channelScope == "" && u.highestCap == ""
}

// FromReadOnly projects a typed read-only principal into the
// runtime UntypedPrincipal form. Engineers use this when handing a
// principal to the runtime registry (e.g., when an agent loop is
// dispatching a model-issued tool call).
func FromReadOnly(p PrincipalContext[CapReadOnly]) UntypedPrincipal {
	return untypedFrom(p.agentID, p.userScope, p.accountScope, p.channelScope, "read_only")
}

// FromWritesLocal projects a typed writes-local principal.
func FromWritesLocal(p PrincipalContext[CapWritesLocal]) UntypedPrincipal {
	return untypedFrom(p.agentID, p.userScope, p.accountScope, p.channelScope, "writes_local")
}

// FromWritesState projects a typed writes-state principal.
func FromWritesState(p PrincipalContext[CapWritesState]) UntypedPrincipal {
	return untypedFrom(p.agentID, p.userScope, p.accountScope, p.channelScope, "writes_state")
}

// FromWritesChain projects a typed writes-chain principal.
func FromWritesChain(p PrincipalContext[CapWritesChain]) UntypedPrincipal {
	return untypedFrom(p.agentID, p.userScope, p.accountScope, p.channelScope, "writes_chain")
}

// FromWritesWallet projects a typed writes-wallet principal.
func FromWritesWallet(p PrincipalContext[CapWritesWallet]) UntypedPrincipal {
	return untypedFrom(p.agentID, p.userScope, p.accountScope, p.channelScope, "writes_wallet")
}

// capabilityLadder is the ordered list of capability class names from
// lowest to highest. Used both by untypedFrom (to validate the input)
// and by HasCap (to decide monotonic coverage).
var capabilityLadder = []string{"read_only", "writes_local", "writes_state", "writes_chain", "writes_wallet"}

// capabilityLadderPos returns the ordinal position of a capability
// class in the ladder, or -1 if the class is not a recognized name.
// A -1 return is what lets untypedFrom fail closed on unknown input.
func capabilityLadderPos(cap string) int {
	for i, c := range capabilityLadder {
		if c == cap {
			return i
		}
	}
	return -1
}

// untypedFrom is the shared builder. It is only called by the typed
// From* helpers above, which supply a known-valid highestCap; if an
// unknown cap ever reaches it (contract violation inside the
// package), the builder stores highestCap="" so HasCap returns false
// for every class. Fail closed by design.
func untypedFrom(agentID, userScope, accountScope, channelScope, highestCap string) UntypedPrincipal {
	if capabilityLadderPos(highestCap) < 0 {
		highestCap = ""
	}
	return UntypedPrincipal{
		agentID:      agentID,
		userScope:    userScope,
		accountScope: accountScope,
		channelScope: channelScope,
		highestCap:   highestCap,
	}
}

// Label returns the canonical principal label string.
func (u UntypedPrincipal) Label() string {
	return serializePrincipalLabel(u.agentID, u.userScope, u.accountScope, u.channelScope)
}

// HasCap reports whether the principal holds at least the requested
// capability class. The ladder is monotonic: writes_wallet implies
// writes_chain implies writes_state implies writes_local implies
// read_only. An unknown cap always returns false (fail closed).
func (u UntypedPrincipal) HasCap(cap string) bool {
	wantPos := capabilityLadderPos(cap)
	if wantPos < 0 {
		return false
	}
	havePos := capabilityLadderPos(u.highestCap)
	if havePos < 0 {
		return false
	}
	return havePos >= wantPos
}

// ErrInsufficientCap is the runtime error returned by the dispatch
// adapter when a principal lacks the required capability class.
var ErrInsufficientCap = errors.New("principal: insufficient capability for tool")

// ErrZeroPrincipal is returned when a zero-valued UntypedPrincipal
// reaches the runtime adapter. Zero principals are never legitimate
// and usually indicate a bug in the caller's construction path.
var ErrZeroPrincipal = errors.New("principal: zero-valued principal cannot dispatch tool")

// ArgDecoder is the callback an adapter uses to turn a raw
// map[string]any into a typed T. Registered by the engineer at
// tool-registration time via AdaptXxx helpers (see below).
type ArgDecoder[T any] func(raw map[string]any) (T, error)

// AdaptReadOnly wraps a TypedTool[CapReadOnly, T] into a Runnable
// that the runtime tool registry can hold.
func AdaptReadOnly[T any](
	tool TypedTool[CapReadOnly, T],
	decode ArgDecoder[T],
) Runnable {
	return &readOnlyAdapter[T]{tool: tool, decode: decode}
}

// AdaptWritesLocal wraps a TypedTool[CapWritesLocal, T].
func AdaptWritesLocal[T any](
	tool TypedTool[CapWritesLocal, T],
	decode ArgDecoder[T],
) Runnable {
	return &writesLocalAdapter[T]{tool: tool, decode: decode}
}

// AdaptWritesState wraps a TypedTool[CapWritesState, T].
func AdaptWritesState[T any](
	tool TypedTool[CapWritesState, T],
	decode ArgDecoder[T],
) Runnable {
	return &writesStateAdapter[T]{tool: tool, decode: decode}
}

// AdaptWritesChain wraps a TypedTool[CapWritesChain, T].
func AdaptWritesChain[T any](
	tool TypedTool[CapWritesChain, T],
	decode ArgDecoder[T],
) Runnable {
	return &writesChainAdapter[T]{tool: tool, decode: decode}
}

// AdaptWritesWallet wraps a TypedTool[CapWritesWallet, T]. The
// runtime cap check is especially load-bearing here because a
// model-issued call path cannot be compile-time checked.
func AdaptWritesWallet[T any](
	tool TypedTool[CapWritesWallet, T],
	decode ArgDecoder[T],
) Runnable {
	return &writesWalletAdapter[T]{tool: tool, decode: decode}
}

// Runnable is the exported non-generic view the runtime registry
// holds. It is the bridge between the compile-time-typed world of
// engineers and the runtime-dispatched world of the model.
type Runnable interface {
	Name() string
	Description() string
	RequiredEffectClass() string
	RunUntyped(ctx context.Context, untyped UntypedPrincipal, args map[string]any) (Result, error)
}

// guardDispatch performs the common pre-dispatch checks shared by
// every adapter: principal non-zero, has required cap. Returning
// structured errors lets the retry layer distinguish "principal
// missing" from "principal lacks cap".
func guardDispatch(toolName string, untyped UntypedPrincipal, requiredCap string) error {
	if untyped.IsZero() {
		return fmt.Errorf("%w: %s", ErrZeroPrincipal, toolName)
	}
	if !untyped.HasCap(requiredCap) {
		return fmt.Errorf("%w: %s requires %s", ErrInsufficientCap, toolName, requiredCap)
	}
	return nil
}

type readOnlyAdapter[T any] struct {
	tool   TypedTool[CapReadOnly, T]
	decode ArgDecoder[T]
}

func (a *readOnlyAdapter[T]) Name() string                { return a.tool.Name() }
func (a *readOnlyAdapter[T]) Description() string         { return a.tool.Description() }
func (a *readOnlyAdapter[T]) RequiredEffectClass() string { return "read_only" }
func (a *readOnlyAdapter[T]) RunUntyped(
	ctx context.Context,
	untyped UntypedPrincipal,
	args map[string]any,
) (Result, error) {
	if err := guardDispatch(a.tool.Name(), untyped, "read_only"); err != nil {
		return Result{}, err
	}
	typed, err := a.decode(args)
	if err != nil {
		return Result{}, fmt.Errorf("%s: arg decode failed: %w", a.tool.Name(), err)
	}
	p := PrincipalContext[CapReadOnly]{
		agentID: untyped.agentID, userScope: untyped.userScope,
		accountScope: untyped.accountScope, channelScope: untyped.channelScope,
	}
	return a.tool.Execute(ctx, p, typed)
}

type writesLocalAdapter[T any] struct {
	tool   TypedTool[CapWritesLocal, T]
	decode ArgDecoder[T]
}

func (a *writesLocalAdapter[T]) Name() string                { return a.tool.Name() }
func (a *writesLocalAdapter[T]) Description() string         { return a.tool.Description() }
func (a *writesLocalAdapter[T]) RequiredEffectClass() string { return "writes_local" }
func (a *writesLocalAdapter[T]) RunUntyped(
	ctx context.Context,
	untyped UntypedPrincipal,
	args map[string]any,
) (Result, error) {
	if err := guardDispatch(a.tool.Name(), untyped, "writes_local"); err != nil {
		return Result{}, err
	}
	typed, err := a.decode(args)
	if err != nil {
		return Result{}, fmt.Errorf("%s: arg decode failed: %w", a.tool.Name(), err)
	}
	p := PrincipalContext[CapWritesLocal]{
		agentID: untyped.agentID, userScope: untyped.userScope,
		accountScope: untyped.accountScope, channelScope: untyped.channelScope,
	}
	return a.tool.Execute(ctx, p, typed)
}

type writesStateAdapter[T any] struct {
	tool   TypedTool[CapWritesState, T]
	decode ArgDecoder[T]
}

func (a *writesStateAdapter[T]) Name() string                { return a.tool.Name() }
func (a *writesStateAdapter[T]) Description() string         { return a.tool.Description() }
func (a *writesStateAdapter[T]) RequiredEffectClass() string { return "writes_state" }
func (a *writesStateAdapter[T]) RunUntyped(
	ctx context.Context,
	untyped UntypedPrincipal,
	args map[string]any,
) (Result, error) {
	if err := guardDispatch(a.tool.Name(), untyped, "writes_state"); err != nil {
		return Result{}, err
	}
	typed, err := a.decode(args)
	if err != nil {
		return Result{}, fmt.Errorf("%s: arg decode failed: %w", a.tool.Name(), err)
	}
	p := PrincipalContext[CapWritesState]{
		agentID: untyped.agentID, userScope: untyped.userScope,
		accountScope: untyped.accountScope, channelScope: untyped.channelScope,
	}
	return a.tool.Execute(ctx, p, typed)
}

type writesChainAdapter[T any] struct {
	tool   TypedTool[CapWritesChain, T]
	decode ArgDecoder[T]
}

func (a *writesChainAdapter[T]) Name() string                { return a.tool.Name() }
func (a *writesChainAdapter[T]) Description() string         { return a.tool.Description() }
func (a *writesChainAdapter[T]) RequiredEffectClass() string { return "writes_chain" }
func (a *writesChainAdapter[T]) RunUntyped(
	ctx context.Context,
	untyped UntypedPrincipal,
	args map[string]any,
) (Result, error) {
	if err := guardDispatch(a.tool.Name(), untyped, "writes_chain"); err != nil {
		return Result{}, err
	}
	typed, err := a.decode(args)
	if err != nil {
		return Result{}, fmt.Errorf("%s: arg decode failed: %w", a.tool.Name(), err)
	}
	p := PrincipalContext[CapWritesChain]{
		agentID: untyped.agentID, userScope: untyped.userScope,
		accountScope: untyped.accountScope, channelScope: untyped.channelScope,
	}
	return a.tool.Execute(ctx, p, typed)
}

type writesWalletAdapter[T any] struct {
	tool   TypedTool[CapWritesWallet, T]
	decode ArgDecoder[T]
}

func (a *writesWalletAdapter[T]) Name() string                { return a.tool.Name() }
func (a *writesWalletAdapter[T]) Description() string         { return a.tool.Description() }
func (a *writesWalletAdapter[T]) RequiredEffectClass() string { return "writes_wallet" }
func (a *writesWalletAdapter[T]) RunUntyped(
	ctx context.Context,
	untyped UntypedPrincipal,
	args map[string]any,
) (Result, error) {
	if err := guardDispatch(a.tool.Name(), untyped, "writes_wallet"); err != nil {
		return Result{}, err
	}
	typed, err := a.decode(args)
	if err != nil {
		return Result{}, fmt.Errorf("%s: arg decode failed: %w", a.tool.Name(), err)
	}
	p := PrincipalContext[CapWritesWallet]{
		agentID: untyped.agentID, userScope: untyped.userScope,
		accountScope: untyped.accountScope, channelScope: untyped.channelScope,
	}
	return a.tool.Execute(ctx, p, typed)
}
