// Package actionlog implements the write-ahead action ledger from
// R7 §4.4: a two-table SQLite schema that records every side-effecting
// tool invocation so a process crash between "tool prepared" and "tool
// committed" leaves a recoverable record.
//
// The invariant the ledger enforces is: no wallet-moving tool
// executes without a durable `action_intents` row written and
// fsync'd BEFORE the tool dispatches, followed by exactly one
// `action_commits` row written AFTER the tool returns (committed)
// or fails (aborted). On startup, any `action_intents` with no
// matching `action_commits` row is surfaced for manual resolution
// or automated replay — the tool may have signed a real tx and
// crashed before we could record the outcome.
//
// The two-table split is deliberate. A single `action_intents` row
// with a mutable `state` column would lose the ordering information
// between prepared and committed (and in SQLite, UPDATE is not
// guaranteed atomic with the preceding INSERT unless wrapped in an
// explicit transaction with a specific journal mode). Two immutable
// append-only tables plus a foreign key from commits to intents
// give us a strict total order and match the hermes request-scoped
// correlation ID model from `research/hermes-agent/RELEASE_v0.8.0.md:253`.
//
// Concurrency model: a single writer goroutine owns the database
// write path. The public Prepare/Commit/Abort methods push op
// records onto an unbuffered channel and wait for the reply on a
// per-op reply channel. This serializes writes without any locks
// and gives us clean context-cancellation support (if the caller's
// ctx is done before the writer picks up the op, the public method
// returns ctx.Err()).
//
// Durability is the other load-bearing property. The SQLite
// connection is opened with `journal_mode=WAL`, `synchronous=FULL`,
// and a single max-open-conns limit. After each successful write,
// the writer issues a `PRAGMA wal_checkpoint(TRUNCATE)` so the row
// is flushed from the WAL to the main file before the public method
// returns. This is the strongest durability SQLite offers — every
// Prepare() return guarantees the intent row is on stable storage.
package actionlog

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	mcsqlite "modernc.org/sqlite"
)

// SQLite extended result codes for constraint violations. These are
// part of SQLite's stable ABI (https://www.sqlite.org/rescode.html)
// and have not changed since SQLite 3.7. We hard-code the numeric
// values rather than depending on the internal `modernc.org/sqlite/lib`
// package, which is not part of the public API contract.
const (
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
)

// Intent is the record written to `action_intents` before a
// side-effecting tool dispatches. It MUST be durably persisted
// before the tool runs; a crash between the persist and the run is
// the scenario the whole ledger exists to make recoverable.
//
// IntentID is optional — if empty, the ledger generates a random
// 128-bit hex identifier. Callers that need to correlate across
// systems (e.g., a trace_id from another service) should supply
// their own.
//
// Principal is the canonical pkg/principal.Label() form:
// "agent=X;user=Y;account=Z;channel=W". The column is stored as-is
// so ledger audits can filter by user scope without parsing.
//
// EffectClass must be one of the capability-class names defined by
// pkg/principal: "read_only", "writes_local", "writes_state",
// "writes_chain", "writes_wallet". The ledger does not validate the
// string against that list (to avoid a circular import); callers
// should pass the string form of the class that authorized the
// tool dispatch.
type Intent struct {
	IntentID    string
	TraceID     string
	ToolName    string
	ArgsHash    string
	Principal   string
	EffectClass string
}

// Commit records the successful outcome of a prepared intent. It is
// written AFTER the tool has returned.
//
// ExternalIDs is a free-form map serialized to JSON. For a signing
// tool this typically carries `{"tx_hash": "0x...", "rpc_id":
// "alchemy-abc123"}`; for a remote-API tool it might carry a
// request_id. The column is queryable as a single JSON blob; if
// callers need structured access, they should also write the
// relevant key to the trace_id column where it can be joined.
//
// ResultHash is the sha256 of the tool's canonicalized result
// payload. It lets replay machinery check that a given intent
// actually produced the expected result without storing the full
// payload in the ledger.
type Commit struct {
	CommitID    string
	IntentID    string
	ExternalIDs map[string]any
	ResultHash  string
}

// Abort records the failed outcome of a prepared intent. It
// occupies the same slot in the schema as a Commit row; callers
// use either Commit() or Abort() after Prepare(), never both.
//
// ErrorMessage should be a short human-readable summary — full
// error chains live elsewhere in logs. The column is VARCHAR and
// may be truncated by display code.
type Abort struct {
	CommitID     string
	IntentID     string
	ErrorMessage string
}

// OrphanedIntent is a row returned by RecoverOrphans: a prepared
// intent that has no matching commit row. The caller decides what
// to do (re-execute, query the external system for the tx hash,
// surface to the user, manual reconciliation).
type OrphanedIntent struct {
	Intent     Intent
	PreparedAt time.Time
}

// Ledger is the write-ahead action ledger handle. It owns a SQLite
// connection and a single writer goroutine. All public operations
// are safe for concurrent use.
//
// The `state` RWMutex is the enqueue gate. Prepare/Commit/Abort take
// it as a read-lock (they don't exclude each other — the writer
// actor already serializes at the DB level) while pushing an op onto
// the write channel. Close takes it as a write-lock, which blocks
// until all in-flight enqueues finish; after that, every subsequent
// enqueue sees `closedFlag=true` and returns ErrClosed. This gives
// strict happens-before/happens-after semantics between Close and
// any producer — there is no "race window" where a producer can
// slip through after Close has started.
type Ledger struct {
	db      *sql.DB
	writeCh chan writeOp
	done    chan struct{}
	closed  chan struct{}

	state      sync.RWMutex // guards closedFlag and serializes enqueue vs Close
	closedFlag bool
	closeMu    sync.Mutex // serializes concurrent Close calls

	// nowFn is swappable for tests; production uses time.Now.
	nowFn func() time.Time
}

// Options configures a new Ledger. DBPath is the filesystem path to
// the SQLite database (created if absent). WriteQueueDepth is the
// unbuffered-or-buffered channel capacity for pending writes; a
// depth of 0 means synchronous handoff (each caller blocks until the
// writer picks up the op).
type Options struct {
	DBPath          string
	WriteQueueDepth int
	NowFn           func() time.Time
}

// Open creates or attaches to a ledger database at the given path.
// It applies the durability PRAGMAs (WAL, synchronous=FULL), creates
// the schema if absent, and starts the writer goroutine. Callers
// must call Close() exactly once when done.
func Open(opts Options) (*Ledger, error) {
	if opts.DBPath == "" {
		return nil, errors.New("actionlog: DBPath is required")
	}
	// SQLite DSN with pragmas applied at connection-open time. The
	// _journal_mode pragma must be WAL for concurrent readers +
	// single-writer; _synchronous must be FULL so fsync fires on
	// every commit; _busy_timeout gives us a bounded retry window
	// for a brief lock contention without returning SQLITE_BUSY.
	dsn := buildDSN(opts.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("actionlog: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("actionlog: init schema: %w", err)
	}

	nowFn := opts.NowFn
	if nowFn == nil {
		nowFn = time.Now
	}

	l := &Ledger{
		db:      db,
		writeCh: make(chan writeOp, opts.WriteQueueDepth),
		done:    make(chan struct{}),
		closed:  make(chan struct{}),
		nowFn:   nowFn,
	}
	go l.writer()
	return l, nil
}

// buildDSN constructs a modernc.org/sqlite DSN with the durability
// and concurrency pragmas the ledger requires. The DB path is
// URL-safe-encoded through url.URL so that a path containing `?`,
// `#`, `&`, or similar reserved bytes does not get misparsed as DSN
// query separators. Without this escaping, a user whose OTTIE_HOME
// contains one of these characters would open the wrong SQLite file
// or silently lose the pragma configuration.
func buildDSN(dbPath string) string {
	q := url.Values{}
	q.Set("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	u := &url.URL{
		Scheme:   "file",
		Path:     dbPath,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// initSchema creates the two tables plus the indexes recovery and
// audit queries need. Using `IF NOT EXISTS` means Open() is
// idempotent on existing databases. The `idx_intents_prepared_at`
// index supports the RecoverOrphans query's ORDER BY clause so
// orphan recovery stays fast as the intents table grows.
func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS action_intents (
			intent_id    TEXT PRIMARY KEY,
			trace_id     TEXT NOT NULL,
			tool_name    TEXT NOT NULL,
			args_hash    TEXT NOT NULL,
			principal    TEXT NOT NULL,
			effect_class TEXT NOT NULL,
			prepared_at  INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS action_commits (
			commit_id     TEXT PRIMARY KEY,
			intent_id     TEXT NOT NULL UNIQUE REFERENCES action_intents(intent_id),
			status        TEXT NOT NULL CHECK (status IN ('committed','aborted')),
			external_ids  TEXT,
			result_hash   TEXT,
			error_message TEXT,
			completed_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intents_trace ON action_intents(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_intents_principal ON action_intents(principal)`,
		`CREATE INDEX IF NOT EXISTS idx_intents_prepared_at ON action_intents(prepared_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", firstLine(s), err)
		}
	}
	return nil
}

// firstLine trims a SQL statement to its first line for error messages.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// writeKind discriminates the three op types that the writer
// goroutine handles. The writer uses a type switch over this to
// keep the actor loop small.
type writeKind int

const (
	kindPrepare writeKind = iota
	kindCommit
	kindAbort
)

// writeOp is the internal message passed through the actor channel.
// Exactly one of intent/commit/abort is populated per op.
type writeOp struct {
	kind   writeKind
	intent *Intent
	commit *Commit
	abort  *Abort
	reply  chan writeResult
}

// writeResult is what the writer sends back to the caller. For
// Prepare, ID holds the generated intent_id; for Commit/Abort,
// it's unused.
type writeResult struct {
	id  string
	err error
}

// writer is the single actor that owns the database write path.
// It runs until the closed channel fires, then drains any ops that
// were already pushed before Close() and exits. The writeCh is
// never closed — closing it while a concurrent Prepare is in the
// middle of its select would race into a "send on closed channel"
// panic. Signaling via l.closed and draining-in-select is the only
// race-free way to shut down a multi-producer Go channel.
func (l *Ledger) writer() {
	defer close(l.done)
	for {
		select {
		case op := <-l.writeCh:
			op.reply <- l.handleOp(op)
		case <-l.closed:
			// Drain any ops that were already pushed. This
			// non-blocking loop guarantees every queued Prepare/
			// Commit/Abort gets a reply before the writer exits,
			// so no caller is left waiting forever on a reply
			// channel.
			for {
				select {
				case op := <-l.writeCh:
					op.reply <- l.handleOp(op)
				default:
					return
				}
			}
		}
	}
}

// handleOp dispatches a single op. Errors are wrapped with the op
// kind so the caller can tell which phase failed.
func (l *Ledger) handleOp(op writeOp) writeResult {
	switch op.kind {
	case kindPrepare:
		return l.doPrepare(op.intent)
	case kindCommit:
		return l.doCommit(op.commit)
	case kindAbort:
		return l.doAbort(op.abort)
	default:
		return writeResult{err: fmt.Errorf("actionlog: unknown writeKind %d", op.kind)}
	}
}

// doPrepare inserts an action_intents row. It enforces the required
// fields at runtime (the caller is an engineer, but the ledger is
// the last line of defense for the "no signing without a prepared
// row" invariant).
func (l *Ledger) doPrepare(in *Intent) writeResult {
	if in.TraceID == "" {
		return writeResult{err: errors.New("actionlog: Prepare: TraceID required")}
	}
	if in.ToolName == "" {
		return writeResult{err: errors.New("actionlog: Prepare: ToolName required")}
	}
	if in.ArgsHash == "" {
		return writeResult{err: errors.New("actionlog: Prepare: ArgsHash required")}
	}
	if in.Principal == "" {
		return writeResult{err: errors.New("actionlog: Prepare: Principal required")}
	}
	if in.EffectClass == "" {
		return writeResult{err: errors.New("actionlog: Prepare: EffectClass required")}
	}
	id := in.IntentID
	if id == "" {
		generated, err := randomID()
		if err != nil {
			return writeResult{err: fmt.Errorf("actionlog: Prepare: generate id: %w", err)}
		}
		id = "int-" + generated
	}
	_, err := l.db.Exec(
		`INSERT INTO action_intents(intent_id, trace_id, tool_name, args_hash, principal, effect_class, prepared_at)
		 VALUES(?,?,?,?,?,?,?)`,
		id, in.TraceID, in.ToolName, in.ArgsHash, in.Principal, in.EffectClass, l.nowFn().UnixMilli(),
	)
	if err != nil {
		return writeResult{err: fmt.Errorf("actionlog: Prepare: insert: %w", err)}
	}
	// Checkpoint flushes the WAL to the main DB file. If it
	// fails, the row IS still committed in the WAL and visible
	// to readers in the same process — the checkpoint is a
	// durability optimization, not a visibility gate. We log the
	// failure but return the intent_id as success so the caller
	// can finalize the row normally instead of stranding a
	// permanent orphan. Codex R12 flagged the previous behavior
	// (returning the checkpoint error) as a load-bearing defect:
	// when Dispatch got an error from Prepare it had no
	// intent_id to pass to Commit/Abort, leaving the persisted
	// row as an un-finalizable orphan forever.
	if err := l.checkpoint(); err != nil {
		slog.Warn("actionlog: Prepare: checkpoint failed (row is committed in WAL)",
			"intent_id", id, "error", err.Error())
	}
	return writeResult{id: id}
}

// doCommit inserts an action_commits row with status=committed.
// The UNIQUE constraint on intent_id enforces the "exactly one
// commit per intent" invariant at the SQL level; a double-commit
// returns a SQL error that gets wrapped with ErrAlreadyFinalized.
func (l *Ledger) doCommit(c *Commit) writeResult {
	if c.IntentID == "" {
		return writeResult{err: errors.New("actionlog: Commit: IntentID required")}
	}
	externalIDsJSON, err := marshalExternalIDs(c.ExternalIDs)
	if err != nil {
		return writeResult{err: fmt.Errorf("actionlog: Commit: marshal external ids: %w", err)}
	}
	id := c.CommitID
	if id == "" {
		generated, err := randomID()
		if err != nil {
			return writeResult{err: fmt.Errorf("actionlog: Commit: generate id: %w", err)}
		}
		id = "cmt-" + generated
	}
	_, err = l.db.Exec(
		`INSERT INTO action_commits(commit_id, intent_id, status, external_ids, result_hash, completed_at)
		 VALUES(?,?,?,?,?,?)`,
		id, c.IntentID, "committed", externalIDsJSON, c.ResultHash, l.nowFn().UnixMilli(),
	)
	if err != nil {
		return writeResult{err: wrapFinalizationError(err, "Commit")}
	}
	if err := l.checkpoint(); err != nil {
		slog.Warn("actionlog: Commit: checkpoint failed (row is committed in WAL)",
			"commit_id", id, "intent_id", c.IntentID, "error", err.Error())
	}
	return writeResult{id: id}
}

// doAbort inserts an action_commits row with status=aborted.
// Shares the UNIQUE intent_id constraint with doCommit, so a double
// finalization returns ErrAlreadyFinalized regardless of whether
// the second call is an Abort or a Commit.
func (l *Ledger) doAbort(a *Abort) writeResult {
	if a.IntentID == "" {
		return writeResult{err: errors.New("actionlog: Abort: IntentID required")}
	}
	id := a.CommitID
	if id == "" {
		generated, err := randomID()
		if err != nil {
			return writeResult{err: fmt.Errorf("actionlog: Abort: generate id: %w", err)}
		}
		id = "cmt-" + generated
	}
	_, err := l.db.Exec(
		`INSERT INTO action_commits(commit_id, intent_id, status, error_message, completed_at)
		 VALUES(?,?,?,?,?)`,
		id, a.IntentID, "aborted", a.ErrorMessage, l.nowFn().UnixMilli(),
	)
	if err != nil {
		return writeResult{err: wrapFinalizationError(err, "Abort")}
	}
	if err := l.checkpoint(); err != nil {
		slog.Warn("actionlog: Abort: checkpoint failed (row is committed in WAL)",
			"commit_id", id, "intent_id", a.IntentID, "error", err.Error())
	}
	return writeResult{id: id}
}

// checkpoint flushes the WAL to the main database file. In
// synchronous=FULL mode, SQLite already fsyncs every transaction,
// but the TRUNCATE variant ensures the WAL file does not grow
// unboundedly and that cross-connection readers (e.g., recovery
// queries on a fresh connection) see the row immediately.
func (l *Ledger) checkpoint() error {
	_, err := l.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// marshalExternalIDs converts a map to canonical JSON, returning
// NULL for an empty/nil map so the column stays NULL in that case.
func marshalExternalIDs(m map[string]any) (sql.NullString, error) {
	if len(m) == 0 {
		return sql.NullString{}, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: string(b), Valid: true}, nil
}

// randomID returns 16 bytes of crypto-random hex as the suffix of a
// generated ID. 128 bits is sufficient uniqueness for the ledger's
// lifetime even at sustained high throughput.
func randomID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// ErrAlreadyFinalized is returned by Commit/Abort when the intent
// already has a commit row — either from a previous Commit or a
// previous Abort. The retry layer should treat this as a
// non-retriable condition.
var ErrAlreadyFinalized = errors.New("actionlog: intent already finalized")

// ErrIntentNotFound is returned by Commit/Abort when the intent_id
// does not exist in action_intents. The FOREIGN KEY check fires
// on insert; we surface it as a distinct error so callers can
// distinguish "programmer bug" from "already committed".
var ErrIntentNotFound = errors.New("actionlog: intent not found")

// wrapFinalizationError inspects a SQLite error and returns a
// structured error that the caller can type-switch on. The primary
// path uses modernc.org/sqlite's typed Error with Code() lookup
// against the documented constraint-violation extended codes
// (SQLITE_CONSTRAINT_UNIQUE=2067, SQLITE_CONSTRAINT_FOREIGNKEY=787);
// this is version-independent because the codes are part of
// SQLite's stable ABI. The substring fallback only fires if the
// driver returns something other than a *mcsqlite.Error (e.g., a
// wrapped network error from a future driver version).
func wrapFinalizationError(err error, op string) error {
	if err == nil {
		return nil
	}
	var se *mcsqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqliteConstraintUnique:
			return fmt.Errorf("actionlog: %s: %w: %v", op, ErrAlreadyFinalized, err)
		case sqliteConstraintForeignKey:
			return fmt.Errorf("actionlog: %s: %w: %v", op, ErrIntentNotFound, err)
		}
	}
	// Fallback: substring match in case the driver ever wraps the
	// error differently or an out-of-tree test harness supplies a
	// plain error string.
	msg := err.Error()
	switch {
	case containsAny(msg, "UNIQUE constraint failed", "constraint failed: action_commits.intent_id"):
		return fmt.Errorf("actionlog: %s: %w: %v", op, ErrAlreadyFinalized, err)
	case containsAny(msg, "FOREIGN KEY constraint failed"):
		return fmt.Errorf("actionlog: %s: %w: %v", op, ErrIntentNotFound, err)
	}
	return fmt.Errorf("actionlog: %s: insert: %w", op, err)
}

// containsAny reports whether s contains any of the substrings.
// A hand-rolled helper to avoid importing strings in a tight file.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

// contains is a minimal substring check. Kept here so error
// wrapping has no external dependency.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ErrClosed is returned by Prepare/Commit/Abort after Close has been
// called. It is a sentinel so callers can distinguish "ledger shut
// down" from "write failed" without substring matching.
var ErrClosed = errors.New("actionlog: ledger closed")

// enqueue is the shared write path for Prepare/Commit/Abort. It
// takes the state lock as a reader, fast-fails with ErrClosed if
// Close has run, and otherwise pushes the op onto the write channel.
// The Close method takes the state lock as a writer, which blocks
// until every in-flight enqueue releases its read-lock — this gives
// strict happens-before semantics between Close and any producer.
// A producer that wins the race against Close finishes its send;
// any producer that starts after Close wins the write-lock sees
// closedFlag=true and returns ErrClosed. There is no "both select
// cases ready simultaneously" ambiguity because the send itself is
// guarded by the read-lock.
func (l *Ledger) enqueue(ctx context.Context, op writeOp) error {
	l.state.RLock()
	if l.closedFlag {
		l.state.RUnlock()
		return ErrClosed
	}
	select {
	case l.writeCh <- op:
		l.state.RUnlock()
		return nil
	case <-ctx.Done():
		l.state.RUnlock()
		return ctx.Err()
	}
}

// Prepare publishes an intent to the writer actor and waits for the
// reply. Returns the persisted intent_id (either the caller-supplied
// value or a generated one) and an error if persistence failed.
// The returned intent_id is the only valid handle for a subsequent
// Commit or Abort.
//
// If ctx is done before the writer picks up the op, Prepare returns
// ctx.Err() without writing anything. If ctx is done while the
// writer is executing the insert, the insert may have already
// succeeded — RecoverOrphans will surface it at the next startup.
// If Close has already been called, Prepare returns ErrClosed.
func (l *Ledger) Prepare(ctx context.Context, intent Intent) (string, error) {
	reply := make(chan writeResult, 1)
	if err := l.enqueue(ctx, writeOp{kind: kindPrepare, intent: &intent, reply: reply}); err != nil {
		return "", err
	}
	select {
	case r := <-reply:
		return r.id, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Commit publishes a committed outcome for a previously-prepared
// intent. Returns ErrAlreadyFinalized if the intent has already been
// Commit-ed or Abort-ed, ErrIntentNotFound if the intent_id does not
// exist, or ErrClosed if the ledger has been shut down.
func (l *Ledger) Commit(ctx context.Context, c Commit) error {
	reply := make(chan writeResult, 1)
	if err := l.enqueue(ctx, writeOp{kind: kindCommit, commit: &c, reply: reply}); err != nil {
		return err
	}
	select {
	case r := <-reply:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Abort publishes an aborted outcome for a previously-prepared
// intent. Same error semantics as Commit.
func (l *Ledger) Abort(ctx context.Context, a Abort) error {
	reply := make(chan writeResult, 1)
	if err := l.enqueue(ctx, writeOp{kind: kindAbort, abort: &a, reply: reply}); err != nil {
		return err
	}
	select {
	case r := <-reply:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RecoverOrphans returns every intent that has no matching commit
// row. This is the query the agent loop runs at startup to find
// prepared-but-not-finalized intents left over from a crash.
//
// The query uses a LEFT JOIN with NULL check rather than a NOT
// EXISTS subquery — both produce the same plan on SQLite but the
// LEFT JOIN form is more obviously correct at a glance.
//
// Orphans are returned in stable order: oldest prepared_at first,
// with SQLite's rowid as a tiebreaker for intents prepared in the
// same millisecond. Without the rowid tiebreaker, concurrent
// prepares that happen to land in the same millisecond would come
// back in arbitrary order, which breaks replay determinism.
//
// The returned slice is always non-nil, even on an empty database.
// A nil slice encodes differently in JSON (`null` vs `[]`) and the
// difference can silently break downstream consumers that expect an
// array.
//
// The caller is responsible for deciding what to do with each
// orphan — automated reconciliation, manual review, or replay —
// because the safe action depends on the tool that was running.
func (l *Ledger) RecoverOrphans(ctx context.Context) ([]OrphanedIntent, error) {
	const q = `
		SELECT i.intent_id, i.trace_id, i.tool_name, i.args_hash,
		       i.principal, i.effect_class, i.prepared_at
		  FROM action_intents i
		  LEFT JOIN action_commits c ON c.intent_id = i.intent_id
		 WHERE c.commit_id IS NULL
		 ORDER BY i.prepared_at ASC, i.rowid ASC
	`
	rows, err := l.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("actionlog: RecoverOrphans: query: %w", err)
	}
	defer rows.Close()

	out := make([]OrphanedIntent, 0)
	for rows.Next() {
		var (
			in         Intent
			preparedMs int64
		)
		if err := rows.Scan(
			&in.IntentID, &in.TraceID, &in.ToolName, &in.ArgsHash,
			&in.Principal, &in.EffectClass, &preparedMs,
		); err != nil {
			return nil, fmt.Errorf("actionlog: RecoverOrphans: scan: %w", err)
		}
		out = append(out, OrphanedIntent{
			Intent:     in,
			PreparedAt: time.UnixMilli(preparedMs),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("actionlog: RecoverOrphans: rows: %w", err)
	}
	return out, nil
}

// Close shuts down the writer goroutine and closes the database
// connection. It is safe to call multiple times; subsequent calls
// return nil. Any in-flight Prepare/Commit/Abort calls whose op is
// already on the channel will be processed before the writer exits,
// but new calls after Close begins return ErrClosed.
//
// The shutdown sequence is:
//
//  1. Take `state.Lock()` — blocks until every in-flight enqueue
//     has released its read-lock. This is the fence that gives
//     strict happens-before/happens-after between Close and any
//     producer.
//  2. Set `closedFlag = true` so any future enqueue under the
//     read-lock sees it and returns ErrClosed.
//  3. Close `l.closed` to signal the writer actor.
//  4. Drop `state.Lock()`.
//  5. Wait on `<-l.done` for the writer actor to drain queued ops
//     and exit.
//  6. Close the underlying SQLite database.
//
// The shutdown signals only through `l.closed`; writeCh is
// deliberately NEVER closed because a concurrent producer that had
// already passed the state check might be holding the read-lock
// and about to send — closing the channel would produce a
// "send on closed channel" panic. The drain-on-signal pattern in
// the writer handles remaining ops safely.
func (l *Ledger) Close() error {
	l.closeMu.Lock()
	defer l.closeMu.Unlock()

	// Fence against producers. All in-flight enqueues must finish
	// before Close can proceed; new enqueues after this lock is
	// released will observe closedFlag=true and return ErrClosed.
	l.state.Lock()
	if l.closedFlag {
		l.state.Unlock()
		return nil
	}
	l.closedFlag = true
	close(l.closed)
	l.state.Unlock()

	<-l.done
	return l.db.Close()
}
