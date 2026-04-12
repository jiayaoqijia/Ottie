package tools

import "context"

// Tool is the interface that all tools must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, args map[string]any) *ToolResult
}

// EffectClass tags a tool with the blast-radius class of its
// side effects. It is the string form of the five pkg/principal
// capability markers. When a tool implements EffectClassifier and
// returns a non-read-only class, the R11 agent-loop wiring wraps
// its dispatch in a pkg/actionlog Prepare / Commit / Abort cycle
// so a process crash between dispatch and result persistence can
// be recovered on restart.
//
// Keeping the class as a string (not the pkg/principal type union)
// lets pkg/tools avoid an import cycle back into pkg/principal —
// the two systems converge at the ledger boundary where both
// speak the same vocabulary.
type EffectClass string

const (
	// EffectReadOnly is the default class for tools that do not
	// implement EffectClassifier. Tools in this class do not get
	// wrapped by the action ledger.
	EffectReadOnly EffectClass = "read_only"

	// EffectWritesLocal means the tool modifies workspace,
	// filesystem, memory, or session state but nothing beyond
	// the local agent instance.
	EffectWritesLocal EffectClass = "writes_local"

	// EffectWritesState means the tool modifies agent-internal
	// state like the skill index, routing config, or registry
	// — narrower than WritesLocal because it could change
	// future agent behavior.
	EffectWritesState EffectClass = "writes_state"

	// EffectWritesChain means the tool submits state-changing
	// operations to off-chain services that route through
	// on-chain protocols (DEX quotes, cast posts, etc.).
	EffectWritesChain EffectClass = "writes_chain"

	// EffectWritesWallet means the tool can sign transactions
	// with the user's private key. Highest-privilege class.
	EffectWritesWallet EffectClass = "writes_wallet"
)

// IsSideEffecting reports whether a class warrants action-ledger
// wrapping. Read-only tools are the majority of the tool set and
// do not need Prepare/Commit rows.
func (c EffectClass) IsSideEffecting() bool {
	return c != "" && c != EffectReadOnly
}

// EffectClassifier is an optional interface tools can implement
// to declare their effect class. Tools that do not implement it
// are treated as read-only and bypass the action ledger.
type EffectClassifier interface {
	EffectClass() EffectClass
}

// ClassOf returns a tool's effect class via the optional
// EffectClassifier interface, or EffectReadOnly if the tool does
// not classify itself. Centralized here so the loop code has one
// place to do the type assertion.
//
// Defends against a nil Tool interface AND a typed-nil concrete
// tool whose EffectClass() method panics on a nil receiver: if
// the input is nil, or the classifier's method would panic, we
// return EffectReadOnly so the caller's dispatch path bypasses
// the ledger. Codex R12 flagged the panic-on-typed-nil path as
// a defensive-hardening gap.
func ClassOf(t Tool) (cls EffectClass) {
	if t == nil {
		return EffectReadOnly
	}
	defer func() {
		if r := recover(); r != nil {
			// Typed-nil classifier or any other panic — fall
			// back to read-only so the dispatch path never
			// crashes on a broken tool.
			cls = EffectReadOnly
		}
	}()
	if ec, ok := t.(EffectClassifier); ok {
		class := ec.EffectClass()
		if class != "" {
			return class
		}
	}
	return EffectReadOnly
}

// --- Request-scoped tool context (channel / chatID) ---
//
// Carried via context.Value so that concurrent tool calls each receive
// their own immutable copy — no mutable state on singleton tool instances.
//
// Keys are unexported pointer-typed vars — guaranteed collision-free,
// and only accessible through the helper functions below.

type toolCtxKey struct{ name string }

var (
	ctxKeyChannel = &toolCtxKey{"channel"}
	ctxKeyChatID  = &toolCtxKey{"chatID"}
)

// WithToolContext returns a child context carrying channel and chatID.
func WithToolContext(ctx context.Context, channel, chatID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyChannel, channel)
	ctx = context.WithValue(ctx, ctxKeyChatID, chatID)
	return ctx
}

// ToolChannel extracts the channel from ctx, or "" if unset.
func ToolChannel(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChannel).(string)
	return v
}

// ToolChatID extracts the chatID from ctx, or "" if unset.
func ToolChatID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyChatID).(string)
	return v
}

// AsyncCallback is a function type that async tools use to notify completion.
// When an async tool finishes its work, it calls this callback with the result.
//
// The ctx parameter allows the callback to be canceled if the agent is shutting down.
// The result parameter contains the tool's execution result.
type AsyncCallback func(ctx context.Context, result *ToolResult)

// AsyncExecutor is an optional interface that tools can implement to support
// asynchronous execution with completion callbacks.
//
// Unlike the old AsyncTool pattern (SetCallback + Execute), AsyncExecutor
// receives the callback as a parameter of ExecuteAsync. This eliminates the
// data race where concurrent calls could overwrite each other's callbacks
// on a shared tool instance.
//
// This is useful for:
//   - Long-running operations that shouldn't block the agent loop
//   - Subagent spawns that complete independently
//   - Background tasks that need to report results later
//
// Example:
//
//	func (t *SpawnTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
//	    go func() {
//	        result := t.runSubagent(ctx, args)
//	        if cb != nil { cb(ctx, result) }
//	    }()
//	    return AsyncResult("Subagent spawned, will report back")
//	}
type AsyncExecutor interface {
	Tool
	// ExecuteAsync runs the tool asynchronously. The callback cb will be
	// invoked (possibly from another goroutine) when the async operation
	// completes. cb is guaranteed to be non-nil by the caller (registry).
	ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult
}

func ToolToSchema(tool Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        tool.Name(),
			"description": tool.Description(),
			"parameters":  tool.Parameters(),
		},
	}
}
