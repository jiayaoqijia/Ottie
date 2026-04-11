// Package acs coordinates the three R6/R7 Adaptive Context System
// stores — pkg/principal (capability typing), pkg/actionlog
// (write-ahead side-effect ledger), and pkg/execmanifest
// (per-turn execution manifest) — behind a single Bundle handle
// that the agent loop can take as an optional dependency.
//
// The coordinator exists so the agent loop deals with ONE
// lifecycle (Open/Close) and ONE configuration surface, not three.
// It also exposes cross-package operations — e.g., RecoverOrphans
// joins actionlog orphans with their execmanifest context so the
// caller can see the full replay story for a prepared-but-not-
// committed side effect.
//
// The Bundle is optional. When config disables ACS, the agent
// loop holds a nil *Bundle and every hook point in the loop is
// guarded by `if bundle != nil { ... }`. The ACS-off path must be
// bit-for-bit identical to the pre-R11 agent loop, so existing
// tests do not regress.
//
// Storage layout: each of the three stores gets its own SQLite
// file under `DBDir`:
//
//	<DBDir>/actionlog.db
//	<DBDir>/execmanifest.db
//	(pkg/principal has no SQLite state — it's a library.)
//
// Separate DB files let each package manage its own schema
// migrations independently and avoid schema-migration coupling
// between the R6/R7 subsystems. A future slice can consolidate
// into one file if the operational cost of three files outweighs
// the migration-independence benefit.
package acs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/actionlog"
	"github.com/jiayaoqijia/ottie/pkg/execmanifest"
)

// Config configures a Bundle. DBDir is the directory where the
// per-store SQLite files are created; callers must ensure the
// directory exists (Open does not create it).
//
// WriteQueueDepth is passed through to both actionlog and
// execmanifest writer actors. A depth of 0 means synchronous
// handoff; typical production use is 8-32.
//
// NowFn is optional; when nil, both stores use time.Now.
type Config struct {
	DBDir           string
	WriteQueueDepth int
	NowFn           func() time.Time
}

// Bundle owns the two SQLite-backed stores (action ledger + exec
// manifest) and exposes them as a single coordinated handle.
//
// Every public operation is safe for concurrent use. Close is
// idempotent and shuts down both stores in order: action ledger
// first (so any pending prepared intents can drain), execmanifest
// second.
type Bundle struct {
	ledger   *actionlog.Ledger
	manifest *execmanifest.Store

	closeMu    sync.Mutex
	closedFlag bool
}

// Options is the on-disk configuration resolved from ACSConfig.
// Exported so tests can construct a deterministic NowFn and
// storage location without touching the production config path.
type Options = Config

// Open creates a Bundle by opening each underlying store. If any
// store fails to open, every already-opened store is closed before
// the error is returned, so the caller never ends up with a
// half-initialized Bundle.
func Open(cfg Config) (*Bundle, error) {
	if cfg.DBDir == "" {
		return nil, errors.New("acs: DBDir is required")
	}

	ledger, err := actionlog.Open(actionlog.Options{
		DBPath:          filepath.Join(cfg.DBDir, "actionlog.db"),
		WriteQueueDepth: cfg.WriteQueueDepth,
		NowFn:           cfg.NowFn,
	})
	if err != nil {
		return nil, fmt.Errorf("acs: open action ledger: %w", err)
	}

	manifest, err := execmanifest.Open(execmanifest.Options{
		DBPath:          filepath.Join(cfg.DBDir, "execmanifest.db"),
		WriteQueueDepth: cfg.WriteQueueDepth,
		NowFn:           cfg.NowFn,
	})
	if err != nil {
		// Partial-open cleanup: close the already-successful
		// ledger before returning the error so the caller is
		// not left with a leaked writer goroutine.
		_ = ledger.Close()
		return nil, fmt.Errorf("acs: open execmanifest: %w", err)
	}

	return &Bundle{
		ledger:   ledger,
		manifest: manifest,
	}, nil
}

// ErrClosed is returned when any Bundle method is called after
// Close. It is a sentinel so callers can distinguish "bundle
// closed" from "underlying store failed."
var ErrClosed = errors.New("acs: bundle closed")

// Ledger returns the underlying action ledger for advanced callers
// that need per-package APIs the coordinator does not wrap. Use
// sparingly; most callers should go through BeginTurn /
// PrepareAction / CommitAction / AbortAction instead.
func (b *Bundle) Ledger() *actionlog.Ledger { return b.ledger }

// Manifest returns the underlying execmanifest store. Same
// caveat as Ledger() — prefer the coordinator methods.
func (b *Bundle) Manifest() *execmanifest.Store { return b.manifest }

// isClosed is a non-blocking check. Used by the coordinator
// wrapper methods to fail fast.
func (b *Bundle) isClosed() bool {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	return b.closedFlag
}

// normalizeClosedErr maps underlying-store ErrClosed sentinels
// back to acs.ErrClosed at the wrapper boundary. This matters
// when a concurrent Close races a wrapper call: the isClosed
// probe may miss the race, delegate to the underlying store, and
// the underlying store's own fence catches it and returns its own
// ErrClosed. Callers using errors.Is(err, acs.ErrClosed) should
// match regardless of which fence caught the race first.
func normalizeClosedErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, actionlog.ErrClosed) || errors.Is(err, execmanifest.ErrClosed) {
		return ErrClosed
	}
	return err
}

// BeginTurn records a new execution manifest row for a turn.
// Thin wrapper around execmanifest.Store.Begin; exists so the
// agent loop never has to import execmanifest directly.
func (b *Bundle) BeginTurn(ctx context.Context, m execmanifest.Manifest) (string, error) {
	if b.isClosed() {
		return "", ErrClosed
	}
	id, err := b.manifest.Begin(ctx, m)
	return id, normalizeClosedErr(err)
}

// RecordLLMCall appends a provider-call row to an existing turn
// manifest. Called once per provider.Chat invocation within a
// turn — retries and fallback-chain attempts each produce their
// own row, so the manifest captures the full LLM-call sequence.
func (b *Bundle) RecordLLMCall(ctx context.Context, c execmanifest.ProviderCall) error {
	if b.isClosed() {
		return ErrClosed
	}
	return normalizeClosedErr(b.manifest.RecordProviderCall(ctx, c))
}

// GetManifest returns the full manifest for a trace_id, bundled
// with all provider-call rows. Thin wrapper around
// execmanifest.Store.Get.
func (b *Bundle) GetManifest(ctx context.Context, traceID string) (execmanifest.FullManifest, error) {
	if b.isClosed() {
		return execmanifest.FullManifest{}, ErrClosed
	}
	full, err := b.manifest.Get(ctx, traceID)
	return full, normalizeClosedErr(err)
}

// MaxTurn returns the largest turn number recorded for a session.
// Agent loop uses this to seed a monotonic per-session turn
// counter that survives restarts and history summarization.
func (b *Bundle) MaxTurn(ctx context.Context, sessionID string) (int, error) {
	if b.isClosed() {
		return 0, ErrClosed
	}
	n, err := b.manifest.MaxTurn(ctx, sessionID)
	return n, normalizeClosedErr(err)
}

// PrepareAction writes a prepared intent to the action ledger
// before a side-effecting tool runs. Thin wrapper around
// actionlog.Ledger.Prepare; returns the persisted intent_id that
// CommitAction/AbortAction will reference.
func (b *Bundle) PrepareAction(ctx context.Context, intent actionlog.Intent) (string, error) {
	if b.isClosed() {
		return "", ErrClosed
	}
	id, err := b.ledger.Prepare(ctx, intent)
	return id, normalizeClosedErr(err)
}

// CommitAction records a successful tool outcome.
func (b *Bundle) CommitAction(ctx context.Context, c actionlog.Commit) error {
	if b.isClosed() {
		return ErrClosed
	}
	return normalizeClosedErr(b.ledger.Commit(ctx, c))
}

// AbortAction records a failed tool outcome.
func (b *Bundle) AbortAction(ctx context.Context, a actionlog.Abort) error {
	if b.isClosed() {
		return ErrClosed
	}
	return normalizeClosedErr(b.ledger.Abort(ctx, a))
}

// EnrichedOrphan is an action-ledger orphan joined with its
// execution manifest context. Callers use RecoverOrphans to see
// prepared-but-not-finalized intents AND the turn-level inputs
// that led to them — the full replay story for each orphan.
//
// When the manifest row is missing (for example, the turn crashed
// between BeginTurn and PrepareAction), ManifestPresent is false
// and Manifest is zero.
type EnrichedOrphan struct {
	Intent           actionlog.OrphanedIntent
	Manifest         execmanifest.FullManifest
	ManifestPresent  bool
	ManifestLookupErr error
}

// RecoverOrphans returns every prepared-but-not-finalized action
// intent, each annotated with its corresponding execution manifest
// (if the manifest was successfully recorded). The caller uses
// this to decide what to do with orphaned side effects: replay,
// reconcile against the external system (e.g., query the chain
// for a tx hash matching the intent's args_hash), or surface to
// the user for manual resolution.
//
// The query runs in two phases: actionlog.RecoverOrphans returns
// the intents, then each intent's trace_id is looked up in
// execmanifest. If a manifest lookup fails with ErrTraceNotFound,
// the orphan is still returned with ManifestPresent=false.
func (b *Bundle) RecoverOrphans(ctx context.Context) ([]EnrichedOrphan, error) {
	if b.isClosed() {
		return nil, ErrClosed
	}
	intents, err := b.ledger.RecoverOrphans(ctx)
	if err != nil {
		if errors.Is(err, actionlog.ErrClosed) {
			return nil, ErrClosed
		}
		return nil, fmt.Errorf("acs: recover orphans: %w", err)
	}
	out := make([]EnrichedOrphan, 0, len(intents))
	for _, intent := range intents {
		enriched := EnrichedOrphan{Intent: intent}
		// The intent.TraceID is the execmanifest trace_id; look
		// it up so the caller sees the full turn context.
		full, mErr := b.manifest.Get(ctx, intent.Intent.TraceID)
		switch {
		case mErr == nil:
			enriched.Manifest = full
			enriched.ManifestPresent = true
		case errors.Is(mErr, execmanifest.ErrTraceNotFound):
			// Manifest is missing — not a hard error. The orphan
			// may have been prepared before BeginTurn landed (a
			// race on process crash) or the ACS config changed
			// between runs. Surface with ManifestPresent=false.
			enriched.ManifestPresent = false
		case errors.Is(mErr, execmanifest.ErrClosed):
			// Shutdown raced the join — surface as a top-level
			// ErrClosed so the caller doesn't bury it in a
			// per-row lookup error that's easy to miss.
			return nil, ErrClosed
		default:
			enriched.ManifestLookupErr = mErr
		}
		out = append(out, enriched)
	}
	return out, nil
}

// Close shuts down both underlying stores. Action ledger closes
// first so any pending prepared intents drain before execmanifest
// shuts down (the order matters if a reconciliation task needs to
// read manifest rows while finalizing ledger rows). Close is
// idempotent; subsequent calls return nil. Both underlying Close
// methods are always called — if the ledger Close fails, the
// manifest is still shut down, and both errors are joined via
// errors.Join so the caller sees every shutdown failure, not just
// the first. This gives the operator complete diagnosis of a
// double-failure shutdown.
func (b *Bundle) Close() error {
	b.closeMu.Lock()
	defer b.closeMu.Unlock()
	if b.closedFlag {
		return nil
	}
	b.closedFlag = true

	var ledgerErr, manifestErr error
	if err := b.ledger.Close(); err != nil {
		ledgerErr = fmt.Errorf("acs: close action ledger: %w", err)
	}
	if err := b.manifest.Close(); err != nil {
		manifestErr = fmt.Errorf("acs: close execmanifest: %w", err)
	}
	return errors.Join(ledgerErr, manifestErr)
}
