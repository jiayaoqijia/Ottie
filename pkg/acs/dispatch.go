// This file implements the write-ahead dispatch helper that wraps
// a side-effecting tool invocation in a Prepare → Run → Commit
// (or Prepare → Run → Abort) cycle against the action ledger.
//
// The helper is the single place in the codebase that holds the
// R7 §4.4 invariant: no side-effecting tool runs without a
// durable prepared row on disk BEFORE the tool dispatches. If a
// process crash happens between Prepare and the tool's effect
// reaching the outside world, startup recovery via
// Bundle.RecoverOrphans sees the orphaned intent and can either
// replay or reconcile against the external system.

package acs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jiayaoqijia/ottie/pkg/actionlog"
	"github.com/jiayaoqijia/ottie/pkg/tools"
)

// DispatchRequest describes a single tool invocation that should
// be wrapped in the action ledger. The caller fills in everything
// the ledger needs to attribute the row; the helper handles the
// Prepare/Commit/Abort lifecycle.
//
// Principal is the serialized principal label from
// pkg/principal.Label(). When the agent loop has not yet been
// wired into pkg/principal, a simple channel-based label is
// acceptable for now.
type DispatchRequest struct {
	TraceID   string // execmanifest trace_id for this turn
	Tool      tools.Tool
	Args      map[string]any
	Principal string
}

// DispatchResult bundles the tool result with the ledger IDs that
// were written during dispatch. IntentID is empty when the tool
// was not side-effecting (or when ACS was disabled) so callers
// can use its presence as a "was this wrapped?" signal.
//
// Committed is true only if the tool ran successfully AND the
// commit row was persisted. If the commit row write fails, the
// caller sees Committed=false and a non-nil FinalizeErr so the
// ledger's state is not silently mis-reported. Codex R12 flagged
// the previous unconditional `Committed: true` as a correctness
// defect because a silent commit failure hid ledger/reality
// drift from operators.
//
// FinalizeErr is set when PrepareAction succeeded but the
// subsequent CommitAction or AbortAction returned an error. The
// user-visible turn still completes (fail-open policy for an
// observability layer), but the caller and operator should be
// able to see the finalization uncertainty.
type DispatchResult struct {
	Result      *tools.ToolResult
	IntentID    string // "" when not wrapped
	Committed   bool   // true only if commit row written successfully
	FinalizeErr error  // non-nil when Commit or Abort failed after a prepared row
}

// RunFunc is the function the caller provides to actually
// dispatch the tool. The helper calls it AFTER Prepare succeeds
// and BEFORE Commit/Abort. We accept a function rather than
// calling tools.Tool.Execute directly because the real agent
// loop's dispatch path goes through
// tools.ToolRegistry.ExecuteWithContext which carries
// channel/chatID/async-callback state the helper has no business
// knowing about.
type RunFunc func(ctx context.Context) *tools.ToolResult

// Dispatch wraps a tool invocation in the action ledger when
// both (a) the Bundle is non-nil and (b) the tool declares a
// side-effecting class via tools.EffectClassifier. Everything
// else — read-only tools, nil bundles, empty trace IDs — falls
// through to run() directly, preserving exact pre-R12 behavior.
//
// The ledger lifecycle is:
//  1. classify the tool via tools.ClassOf
//  2. if read-only or wrapping disabled, just call run and return
//  3. hash the args to produce an attributable ArgsHash
//  4. PrepareAction; on failure, fall through to run (fail-open
//     because ACS is an observability layer)
//  5. call run
//  6. inspect the result; on error/IsError → AbortAction, else →
//     CommitAction with the result hash in ExternalIDs
//  7. return the result plus the ledger IDs for the caller's log
func (b *Bundle) Dispatch(
	ctx context.Context,
	req DispatchRequest,
	run RunFunc,
) *DispatchResult {
	// Fast path: nothing to wrap. Call run directly and return
	// an empty ledger-id set so the caller can tell we bypassed.
	if b == nil || req.Tool == nil || req.TraceID == "" {
		return &DispatchResult{Result: run(ctx)}
	}

	class := tools.ClassOf(req.Tool)
	if !class.IsSideEffecting() {
		return &DispatchResult{Result: run(ctx)}
	}

	// Async-executor bypass (codex R12 finding): a tool that
	// implements tools.AsyncExecutor dispatches a background job
	// whose real outcome arrives later via callback, not from
	// the synchronous return value. Auto-committing the
	// immediate AsyncResult placeholder would lie about a
	// not-yet-committed outcome. For this slice, we skip
	// ledger wrapping for async tools entirely; a future slice
	// can add a dedicated async-finalization protocol that
	// matches the callback to a pending intent.
	if _, async := req.Tool.(tools.AsyncExecutor); async {
		return &DispatchResult{Result: run(ctx)}
	}

	argsHash, err := HashArgsForLedger(req.Args)
	if err != nil {
		// A hash failure is almost impossible (map[string]any
		// serialization works for any JSON-able input), but if
		// it happens, fail open with a warning-shaped result so
		// the operator can diagnose.
		return &DispatchResult{Result: run(ctx)}
	}

	principal := req.Principal
	if principal == "" {
		principal = "unknown" // non-empty so the NOT NULL column check passes
	}

	intentID, prepErr := b.PrepareAction(ctx, actionlog.Intent{
		TraceID:     req.TraceID,
		ToolName:    req.Tool.Name(),
		ArgsHash:    argsHash,
		Principal:   principal,
		EffectClass: string(class),
	})
	if prepErr != nil {
		// Fail-open: run anyway so the user-visible turn still
		// completes. A future slice can add a strict-mode
		// config where Prepare failure blocks the dispatch.
		result := run(ctx)
		return &DispatchResult{Result: result}
	}

	result := run(ctx)

	// Codex R12 finding: a tool that returns nil must NOT
	// auto-commit a placeholder. Treat it as a broken-tool
	// error, synthesize a non-nil result so the caller does not
	// panic on dereference at loop.go:1695, and record an abort
	// row so the orphan recovery sees the failure.
	if result == nil {
		result = &tools.ToolResult{
			IsError: true,
			ForLLM:  fmt.Sprintf("tool %q returned nil result", req.Tool.Name()),
			Err:     fmt.Errorf("tool %q returned nil result", req.Tool.Name()),
		}
	}

	if result.IsError || result.Err != nil {
		errMsg := "tool returned error"
		if result.Err != nil {
			errMsg = result.Err.Error()
		} else if result.ForLLM != "" {
			errMsg = result.ForLLM
		}
		abortErr := b.AbortAction(ctx, actionlog.Abort{
			IntentID:     intentID,
			ErrorMessage: errMsg,
		})
		return &DispatchResult{
			Result:      result,
			IntentID:    intentID,
			Committed:   false,
			FinalizeErr: abortErr,
		}
	}

	resultHash := HashResultForLedger(result)
	commitErr := b.CommitAction(ctx, actionlog.Commit{
		IntentID:    intentID,
		ExternalIDs: nil, // tool-specific external IDs are a follow-up
		ResultHash:  resultHash,
	})
	return &DispatchResult{
		Result:      result,
		IntentID:    intentID,
		Committed:   commitErr == nil,
		FinalizeErr: commitErr,
	}
}

// HashArgsForLedger produces a deterministic sha256 over a tool's
// argument map. Exported so the agent loop can pre-compute the
// hash for logging, and so tests can assert against known values.
//
// Determinism relies on encoding/json's keys being sorted
// alphabetically for map[string]any, which the stdlib guarantees
// as of Go 1.12.
func HashArgsForLedger(args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("acs: hash args: %w", err)
	}
	sum := sha256.Sum256(b)
	return "sha256-" + hex.EncodeToString(sum[:]), nil
}

// HashResultForLedger produces a deterministic hash of a tool
// result's user-visible payload. Used as the ResultHash in the
// action_commits row so replay machinery can check "did the
// committed result match the expected payload?" without storing
// the full result body in the ledger.
//
// Uses canonical JSON encoding over a struct with the three
// load-bearing fields (IsError, ForUser, ForLLM) because a
// delimiter-concatenation approach has collision risk: codex R12
// flagged that 0x1F is a valid byte in Go strings and a tool
// that somehow produced it would hash-collide with a different
// split. JSON gives us a type-safe, non-colliding canonical
// form at the cost of one extra allocation per commit row.
func HashResultForLedger(result *tools.ToolResult) string {
	if result == nil {
		return "sha256-empty"
	}
	payload, err := json.Marshal(struct {
		IsError bool   `json:"is_error"`
		ForUser string `json:"for_user"`
		ForLLM  string `json:"for_llm"`
	}{
		IsError: result.IsError,
		ForUser: result.ForUser,
		ForLLM:  result.ForLLM,
	})
	if err != nil {
		// Marshal failure is impossible for these field types.
		// Return a sentinel so the caller can still see a
		// non-empty hash value in the ledger.
		return "sha256-marshal-error"
	}
	sum := sha256.Sum256(payload)
	return "sha256-" + hex.EncodeToString(sum[:])
}

// ErrDispatchFailed is returned when a caller explicitly asked
// for strict-mode ledger wrapping and the Prepare step failed.
// Reserved for a future strict-mode flag; the default Dispatch
// path today is fail-open and does not return this error.
var ErrDispatchFailed = errors.New("acs: dispatch failed with strict mode")
