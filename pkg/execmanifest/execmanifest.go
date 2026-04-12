// Package execmanifest implements the per-turn execution manifest
// from R6/R7 §4.4: the immutable record of exactly what inputs
// produced a given agent turn, so a turn can be replayed bit-for-bit
// from the ledger.
//
// The execution manifest is the third leg of Ottie's replay story,
// alongside pkg/principal (compile-time authorization) and
// pkg/actionlog (write-ahead side-effect ledger). Together the three
// packages let `ottie replay <trace_id>` reconstruct any signing
// decision: the action ledger tells you which tool was dispatched,
// the manifest tells you the exact prompt hash / tool schema hash /
// skill hash set / MCP server versions / provider request IDs the
// agent used, and the principal guarantees the dispatch was
// type-safe.
//
// Schema: two immutable SQLite tables.
//
//	traces
//	  trace_id          TEXT PRIMARY KEY
//	  session_id        TEXT NOT NULL
//	  turn              INTEGER NOT NULL
//	  prompt_hash       TEXT NOT NULL
//	  tool_schema_hash  TEXT NOT NULL
//	  skill_hashes      TEXT NOT NULL  -- JSON array of sha256
//	  mcp_versions      TEXT NOT NULL  -- JSON map
//	  model_id          TEXT NOT NULL
//	  prompt_epoch      INTEGER NOT NULL
//	  created_at        INTEGER NOT NULL
//	  UNIQUE(session_id, turn)         -- a turn is recorded at most once
//
//	trace_provider_requests
//	  trace_id     TEXT NOT NULL REFERENCES traces
//	  call_seq     INTEGER NOT NULL
//	  request_id   TEXT NOT NULL
//	  model_id     TEXT
//	  recorded_at  INTEGER NOT NULL
//	  PRIMARY KEY (trace_id, call_seq)
//
// The two-table split keeps the core manifest immutable while
// allowing the provider_request_ids list to grow during a turn
// (multiple LLM calls per turn). Both tables are append-only: a row
// is written once and never updated.
//
// Concurrency + durability follow the same pattern as
// pkg/actionlog: single writer actor, RWMutex state fence for
// shutdown, WAL + synchronous=FULL with wal_checkpoint(TRUNCATE)
// after every write.
package execmanifest

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

// SQLite extended result codes for constraint violations. Identical
// to pkg/actionlog — SQLite's stable ABI; not expected to change.
const (
	sqliteConstraintForeignKey = 787  // SQLITE_CONSTRAINT_FOREIGNKEY
	sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
)

// Manifest is the per-turn immutable record. It is written ONCE via
// Begin() at the start of each turn and never mutated.
//
// TraceID is optional on input — Begin() generates a random 128-bit
// hex ID if empty. Callers that need to correlate the manifest with
// an externally-generated trace (e.g., from pkg/actionlog or an
// OpenTelemetry trace) should supply their own.
//
// PromptHash is the sha256 of the exact bytes of the system prompt
// used for this turn. The Ottie prompt-epoch discipline from R4 §14
// guarantees the system prompt is byte-stable within an epoch, so
// this hash is sufficient to look up the prompt from a separate
// prompt-cache store when replaying.
//
// ToolSchemaHash is the sha256 of the canonicalized serialization
// of every registered tool's (name, description, parameter schema).
// It pins the exact tool surface the model saw at turn start so a
// replay cannot silently drift if a tool's schema changes between
// runs.
//
// SkillHashes is a list of sha256 hashes, one per loaded skill file.
// Order is stable (sorted by skill name) so the list itself can be
// compared element-wise.
//
// McpVersions maps MCP server ID → version string. Empty for turns
// that don't use any MCP servers.
//
// ModelID is the canonical model identifier, e.g. "claude-sonnet-4-6".
// A single turn has exactly one primary model; fallback-chain
// invocations to other models are recorded as additional
// ProviderCall rows with their own model_id.
//
// PromptEpoch is the monotonic epoch number from the pkg/agent
// prompt-epoch discipline. Two turns with the same prompt_epoch
// have the same system-prompt bytes.
type Manifest struct {
	TraceID        string
	SessionID      string
	Turn           int
	PromptHash     string
	ToolSchemaHash string
	SkillHashes    []string
	McpVersions    map[string]string
	ModelID        string
	PromptEpoch    int64
}

// ProviderCall is one LLM-provider invocation within a turn. Most
// turns have exactly one (the main model call), but fallback-chain
// invocations and retries produce additional rows. Rows are
// append-only; the (trace_id, call_seq) PK prevents duplicate
// recording.
//
// RequestID is the provider's own correlation ID (e.g., the
// OpenAI or Anthropic request ID from the response header). It is
// the load-bearing value for replaying the exact same call against
// the provider — without it, a replay cannot guarantee byte-
// identical LLM output even with the same inputs.
type ProviderCall struct {
	TraceID   string
	CallSeq   int
	RequestID string
	ModelID   string
}

// FullManifest is the merged view returned by Get/GetBySessionTurn/
// ListBySession. It bundles the immutable trace row with its
// aggregated provider-call rows and the recorded timestamps.
type FullManifest struct {
	Manifest
	CreatedAt     time.Time
	ProviderCalls []ProviderCall
}

// Options configures a new Store. DBPath is the filesystem path to
// the SQLite database (created if absent). WriteQueueDepth is the
// channel capacity for the writer actor's pending queue; a depth of
// 0 means synchronous handoff.
type Options struct {
	DBPath          string
	WriteQueueDepth int
	NowFn           func() time.Time
}

// Store is the execution-manifest handle. Like pkg/actionlog, it
// owns a SQLite connection and a single writer goroutine, and is
// safe for concurrent use.
//
// The state RWMutex is the enqueue gate: Begin/RecordProviderCall
// take it as readers while pushing ops to the writer channel;
// Close takes it as a writer so new enqueues after Close cannot
// slip through. Identical to the pattern in pkg/actionlog.
type Store struct {
	db      *sql.DB
	writeCh chan writeOp
	done    chan struct{}
	closed  chan struct{}

	state      sync.RWMutex
	closedFlag bool
	closeMu    sync.Mutex

	nowFn func() time.Time
}

// Open creates or attaches to a manifest database at the given
// path. It applies the durability PRAGMAs (WAL, synchronous=FULL),
// creates the schema if absent, and starts the writer goroutine.
// Callers must call Close() exactly once when done.
func Open(opts Options) (*Store, error) {
	if opts.DBPath == "" {
		return nil, errors.New("execmanifest: DBPath is required")
	}
	dsn := buildDSN(opts.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("execmanifest: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("execmanifest: init schema: %w", err)
	}

	nowFn := opts.NowFn
	if nowFn == nil {
		nowFn = time.Now
	}

	s := &Store{
		db:      db,
		writeCh: make(chan writeOp, opts.WriteQueueDepth),
		done:    make(chan struct{}),
		closed:  make(chan struct{}),
		nowFn:   nowFn,
	}
	go s.writer()
	return s, nil
}

// buildDSN constructs the sqlite DSN with URL-safe path encoding
// and the durability PRAGMAs the store requires. Same pattern as
// pkg/actionlog.buildDSN — a path containing `?` or `#` would
// otherwise be misparsed as DSN query separators.
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

// initSchema creates the two tables plus the indexes audit and
// replay queries use. Every DDL uses `IF NOT EXISTS` so Open is
// idempotent on existing databases.
func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS traces (
			trace_id         TEXT PRIMARY KEY,
			session_id       TEXT NOT NULL,
			turn             INTEGER NOT NULL,
			prompt_hash      TEXT NOT NULL,
			tool_schema_hash TEXT NOT NULL,
			skill_hashes     TEXT NOT NULL,
			mcp_versions     TEXT NOT NULL,
			model_id         TEXT NOT NULL,
			prompt_epoch     INTEGER NOT NULL,
			created_at       INTEGER NOT NULL,
			UNIQUE(session_id, turn)
		)`,
		`CREATE TABLE IF NOT EXISTS trace_provider_requests (
			trace_id     TEXT NOT NULL REFERENCES traces(trace_id),
			call_seq     INTEGER NOT NULL,
			request_id   TEXT NOT NULL,
			model_id     TEXT,
			recorded_at  INTEGER NOT NULL,
			PRIMARY KEY (trace_id, call_seq)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_session ON traces(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_prompt_epoch ON traces(prompt_epoch)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_created_at ON traces(created_at)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", firstLine(s), err)
		}
	}
	return nil
}

// firstLine trims a SQL statement for error messages.
func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

// --- writer actor ---------------------------------------------------

type writeKind int

const (
	kindBegin writeKind = iota
	kindRecordCall
)

type writeOp struct {
	kind     writeKind
	manifest *Manifest
	call     *ProviderCall
	reply    chan writeResult
}

type writeResult struct {
	id  string
	err error
}

// writer is the single actor that owns the database write path. It
// runs until closed fires, then drains any ops already on writeCh
// and exits. Matches the pkg/actionlog pattern.
func (s *Store) writer() {
	defer close(s.done)
	for {
		select {
		case op := <-s.writeCh:
			op.reply <- s.handleOp(op)
		case <-s.closed:
			for {
				select {
				case op := <-s.writeCh:
					op.reply <- s.handleOp(op)
				default:
					return
				}
			}
		}
	}
}

func (s *Store) handleOp(op writeOp) writeResult {
	switch op.kind {
	case kindBegin:
		return s.doBegin(op.manifest)
	case kindRecordCall:
		return s.doRecordCall(op.call)
	default:
		return writeResult{err: fmt.Errorf("execmanifest: unknown writeKind %d", op.kind)}
	}
}

// doBegin inserts a traces row. Required fields are validated in
// Go before hitting SQL so the error messages name the specific
// field; the NOT NULL declarations in the schema are a second line
// of defense.
func (s *Store) doBegin(m *Manifest) writeResult {
	if m.SessionID == "" {
		return writeResult{err: errors.New("execmanifest: Begin: SessionID required")}
	}
	if m.Turn < 0 {
		return writeResult{err: fmt.Errorf("execmanifest: Begin: Turn must be >= 0 (got %d)", m.Turn)}
	}
	if m.PromptHash == "" {
		return writeResult{err: errors.New("execmanifest: Begin: PromptHash required")}
	}
	if m.ToolSchemaHash == "" {
		return writeResult{err: errors.New("execmanifest: Begin: ToolSchemaHash required")}
	}
	if m.ModelID == "" {
		return writeResult{err: errors.New("execmanifest: Begin: ModelID required")}
	}
	id := m.TraceID
	if id == "" {
		generated, err := randomID()
		if err != nil {
			return writeResult{err: fmt.Errorf("execmanifest: Begin: generate id: %w", err)}
		}
		id = "trc-" + generated
	}
	skillHashesJSON, err := canonicalJSONArray(m.SkillHashes)
	if err != nil {
		return writeResult{err: fmt.Errorf("execmanifest: Begin: marshal skill_hashes: %w", err)}
	}
	mcpVersionsJSON, err := canonicalJSONMap(m.McpVersions)
	if err != nil {
		return writeResult{err: fmt.Errorf("execmanifest: Begin: marshal mcp_versions: %w", err)}
	}

	_, err = s.db.Exec(
		`INSERT INTO traces(trace_id, session_id, turn, prompt_hash, tool_schema_hash,
		                    skill_hashes, mcp_versions, model_id, prompt_epoch, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, m.SessionID, m.Turn, m.PromptHash, m.ToolSchemaHash,
		skillHashesJSON, mcpVersionsJSON, m.ModelID, m.PromptEpoch, s.nowFn().UnixMilli(),
	)
	if err != nil {
		return writeResult{err: wrapBeginError(err)}
	}
	if err := s.checkpoint(); err != nil {
		slog.Warn("execmanifest: Begin: checkpoint failed (row is committed in WAL)",
			"trace_id", id, "error", err.Error())
	}
	return writeResult{id: id}
}

// doRecordCall appends a row to trace_provider_requests. The
// (trace_id, call_seq) PK prevents duplicate recording of the same
// call; the FK to traces returns ErrTraceNotFound if the trace_id
// was never Begin'd.
func (s *Store) doRecordCall(c *ProviderCall) writeResult {
	if c.TraceID == "" {
		return writeResult{err: errors.New("execmanifest: RecordProviderCall: TraceID required")}
	}
	if c.RequestID == "" {
		return writeResult{err: errors.New("execmanifest: RecordProviderCall: RequestID required")}
	}
	if c.CallSeq < 0 {
		return writeResult{err: fmt.Errorf("execmanifest: RecordProviderCall: CallSeq must be >= 0 (got %d)", c.CallSeq)}
	}
	var modelID sql.NullString
	if c.ModelID != "" {
		modelID = sql.NullString{String: c.ModelID, Valid: true}
	}
	_, err := s.db.Exec(
		`INSERT INTO trace_provider_requests(trace_id, call_seq, request_id, model_id, recorded_at)
		 VALUES(?,?,?,?,?)`,
		c.TraceID, c.CallSeq, c.RequestID, modelID, s.nowFn().UnixMilli(),
	)
	if err != nil {
		return writeResult{err: wrapRecordCallError(err)}
	}
	if err := s.checkpoint(); err != nil {
		slog.Warn("execmanifest: RecordProviderCall: checkpoint failed (row is committed in WAL)",
			"trace_id", c.TraceID, "call_seq", c.CallSeq, "error", err.Error())
	}
	return writeResult{}
}

// checkpoint flushes the WAL to the main database file.
func (s *Store) checkpoint() error {
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// --- JSON helpers ---------------------------------------------------

// canonicalJSONArray serializes a string slice as a JSON array.
// A nil slice becomes `[]` (not `null`) so the column always has a
// parseable value and replay code can assume a list.
func canonicalJSONArray(xs []string) (string, error) {
	if xs == nil {
		xs = []string{}
	}
	b, err := json.Marshal(xs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// canonicalJSONMap serializes a string map as a JSON object.
// A nil or empty map becomes `{}`.
func canonicalJSONMap(m map[string]string) (string, error) {
	if m == nil {
		m = map[string]string{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// randomID returns 16 bytes of crypto-random hex, matching the
// pkg/actionlog ID format (128 bits of uniqueness).
func randomID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// --- error wrapping -------------------------------------------------

// ErrTraceAlreadyRecorded is returned by Begin when a trace_id is
// already present — either because the caller reused a trace_id or
// because the same (session_id, turn) was recorded twice.
var ErrTraceAlreadyRecorded = errors.New("execmanifest: trace already recorded")

// ErrTraceNotFound is returned by RecordProviderCall/Get/
// GetBySessionTurn when the referenced trace_id does not exist.
var ErrTraceNotFound = errors.New("execmanifest: trace not found")

// ErrCallAlreadyRecorded is returned by RecordProviderCall when a
// (trace_id, call_seq) pair is already present. Prevents duplicate
// recording of the same LLM call.
var ErrCallAlreadyRecorded = errors.New("execmanifest: provider call already recorded")

// ErrClosed is returned when Begin/RecordProviderCall/Get/
// ListBySession are called after Close has been invoked.
var ErrClosed = errors.New("execmanifest: store closed")

// wrapBeginError inspects a SQLite error from the traces INSERT
// and returns a structured error.
func wrapBeginError(err error) error {
	if err == nil {
		return nil
	}
	var se *mcsqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqliteConstraintUnique, sqliteConstraintPrimaryKey:
			return fmt.Errorf("execmanifest: Begin: %w: %v", ErrTraceAlreadyRecorded, err)
		}
	}
	msg := err.Error()
	if containsAny(msg, "UNIQUE constraint failed", "constraint failed: traces") {
		return fmt.Errorf("execmanifest: Begin: %w: %v", ErrTraceAlreadyRecorded, err)
	}
	return fmt.Errorf("execmanifest: Begin: insert: %w", err)
}

// wrapRecordCallError inspects a SQLite error from the
// trace_provider_requests INSERT.
func wrapRecordCallError(err error) error {
	if err == nil {
		return nil
	}
	var se *mcsqlite.Error
	if errors.As(err, &se) {
		switch se.Code() {
		case sqliteConstraintUnique, sqliteConstraintPrimaryKey:
			return fmt.Errorf("execmanifest: RecordProviderCall: %w: %v", ErrCallAlreadyRecorded, err)
		case sqliteConstraintForeignKey:
			return fmt.Errorf("execmanifest: RecordProviderCall: %w: %v", ErrTraceNotFound, err)
		}
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "UNIQUE constraint failed", "PRIMARY KEY constraint failed"):
		return fmt.Errorf("execmanifest: RecordProviderCall: %w: %v", ErrCallAlreadyRecorded, err)
	case containsAny(msg, "FOREIGN KEY constraint failed"):
		return fmt.Errorf("execmanifest: RecordProviderCall: %w: %v", ErrTraceNotFound, err)
	}
	return fmt.Errorf("execmanifest: RecordProviderCall: insert: %w", err)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if contains(s, sub) {
			return true
		}
	}
	return false
}

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

// --- public write API ------------------------------------------------

// enqueue is the shared write path. Same fence pattern as
// pkg/actionlog: RLock the state mutex, fail fast on closedFlag,
// push to the writer channel while holding the read-lock so Close
// cannot begin until every in-flight enqueue releases.
func (s *Store) enqueue(ctx context.Context, op writeOp) error {
	s.state.RLock()
	if s.closedFlag {
		s.state.RUnlock()
		return ErrClosed
	}
	select {
	case s.writeCh <- op:
		s.state.RUnlock()
		return nil
	case <-ctx.Done():
		s.state.RUnlock()
		return ctx.Err()
	}
}

// Begin records a new trace row. Returns the persisted trace_id
// (caller-supplied or auto-generated). Returns ErrTraceAlreadyRecorded
// if the trace_id or (session_id, turn) pair already exists;
// ErrClosed if the store has been shut down.
func (s *Store) Begin(ctx context.Context, m Manifest) (string, error) {
	reply := make(chan writeResult, 1)
	if err := s.enqueue(ctx, writeOp{kind: kindBegin, manifest: &m, reply: reply}); err != nil {
		return "", err
	}
	select {
	case r := <-reply:
		return r.id, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// RecordProviderCall appends a provider-call row to an existing
// trace. Returns ErrTraceNotFound if the trace_id does not exist;
// ErrCallAlreadyRecorded if (trace_id, call_seq) is a duplicate;
// ErrClosed if the store has been shut down.
func (s *Store) RecordProviderCall(ctx context.Context, c ProviderCall) error {
	reply := make(chan writeResult, 1)
	if err := s.enqueue(ctx, writeOp{kind: kindRecordCall, call: &c, reply: reply}); err != nil {
		return err
	}
	select {
	case r := <-reply:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- read API --------------------------------------------------------

// Get returns the full manifest for a trace_id, including all
// recorded provider calls. Returns ErrTraceNotFound if the trace
// does not exist or ErrClosed if the store has been shut down.
func (s *Store) Get(ctx context.Context, traceID string) (FullManifest, error) {
	if traceID == "" {
		return FullManifest{}, errors.New("execmanifest: Get: traceID required")
	}
	var full FullManifest
	err := s.guardedRead(func() error {
		row := s.db.QueryRowContext(ctx,
			`SELECT trace_id, session_id, turn, prompt_hash, tool_schema_hash,
			        skill_hashes, mcp_versions, model_id, prompt_epoch, created_at
			   FROM traces WHERE trace_id = ?`,
			traceID,
		)
		var e error
		full, e = scanTrace(row, "Get")
		if e != nil {
			return e
		}
		calls, e := s.loadProviderCalls(ctx, traceID, "Get")
		if e != nil {
			return e
		}
		full.ProviderCalls = calls
		return nil
	})
	return full, err
}

// GetBySessionTurn returns the manifest for a (session_id, turn)
// pair. Useful when the caller does not remember the trace_id but
// can address a turn by its session lineage.
func (s *Store) GetBySessionTurn(ctx context.Context, sessionID string, turn int) (FullManifest, error) {
	if sessionID == "" {
		return FullManifest{}, errors.New("execmanifest: GetBySessionTurn: sessionID required")
	}
	var full FullManifest
	err := s.guardedRead(func() error {
		row := s.db.QueryRowContext(ctx,
			`SELECT trace_id, session_id, turn, prompt_hash, tool_schema_hash,
			        skill_hashes, mcp_versions, model_id, prompt_epoch, created_at
			   FROM traces WHERE session_id = ? AND turn = ?`,
			sessionID, turn,
		)
		var e error
		full, e = scanTrace(row, "GetBySessionTurn")
		if e != nil {
			return e
		}
		calls, e := s.loadProviderCalls(ctx, full.TraceID, "GetBySessionTurn")
		if e != nil {
			return e
		}
		full.ProviderCalls = calls
		return nil
	})
	return full, err
}

// MaxTurn returns the largest `turn` value recorded for a session,
// or 0 if the session has no recorded turns. Used by the agent
// loop to initialize a monotonic per-session turn counter that
// survives process restarts and history summarization. Returns
// ErrClosed if the store has been shut down.
//
// This is O(1) in the common case because traces has a composite
// index on session_id (via the UNIQUE(session_id, turn)
// constraint) and SQLite's MAX() on a single column is a direct
// index lookup, not a scan.
func (s *Store) MaxTurn(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, errors.New("execmanifest: MaxTurn: sessionID required")
	}
	var maxTurn int
	err := s.guardedRead(func() error {
		row := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(turn), 0) FROM traces WHERE session_id = ?`,
			sessionID,
		)
		return row.Scan(&maxTurn)
	})
	if err != nil {
		return 0, err
	}
	return maxTurn, nil
}

// ListBySession returns every manifest for a session in turn order.
// Returns a non-nil empty slice if the session has no recorded
// turns. Each FullManifest's ProviderCalls slice is hydrated.
func (s *Store) ListBySession(ctx context.Context, sessionID string) ([]FullManifest, error) {
	if sessionID == "" {
		return nil, errors.New("execmanifest: ListBySession: sessionID required")
	}
	var out []FullManifest
	err := s.guardedRead(func() error {
		rows, err := s.db.QueryContext(ctx,
			`SELECT trace_id, session_id, turn, prompt_hash, tool_schema_hash,
			        skill_hashes, mcp_versions, model_id, prompt_epoch, created_at
			   FROM traces WHERE session_id = ? ORDER BY turn ASC`,
			sessionID,
		)
		if err != nil {
			return fmt.Errorf("execmanifest: ListBySession: query: %w", err)
		}
		defer rows.Close()

		out = make([]FullManifest, 0)
		for rows.Next() {
			full, err := scanTraceFromRows(rows, "ListBySession")
			if err != nil {
				return err
			}
			out = append(out, full)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("execmanifest: ListBySession: rows: %w", err)
		}
		// Hydrate provider calls for each manifest. A per-trace
		// query is simpler than a single JOIN, and session length
		// bounds N at typical Ottie use (tens of turns per
		// session). Codex R10 noted this N+1 pattern is
		// acceptable here under SetMaxOpenConns(1).
		for i := range out {
			calls, err := s.loadProviderCalls(ctx, out[i].TraceID, "ListBySession")
			if err != nil {
				return err
			}
			out[i].ProviderCalls = calls
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// loadProviderCalls returns every provider call for a trace,
// ordered by call_seq ascending. Always returns a non-nil slice.
// The op string is threaded through into error messages so the
// caller can tell whether a failure happened under Get vs
// GetBySessionTurn vs ListBySession without decoding stack traces.
func (s *Store) loadProviderCalls(ctx context.Context, traceID, op string) ([]ProviderCall, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT trace_id, call_seq, request_id, COALESCE(model_id, '')
		   FROM trace_provider_requests
		  WHERE trace_id = ?
		  ORDER BY call_seq ASC`,
		traceID,
	)
	if err != nil {
		return nil, fmt.Errorf("execmanifest: %s: loadProviderCalls query: %w", op, err)
	}
	defer rows.Close()

	out := make([]ProviderCall, 0)
	for rows.Next() {
		var c ProviderCall
		if err := rows.Scan(&c.TraceID, &c.CallSeq, &c.RequestID, &c.ModelID); err != nil {
			return nil, fmt.Errorf("execmanifest: %s: loadProviderCalls scan: %w", op, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("execmanifest: %s: loadProviderCalls rows: %w", op, err)
	}
	return out, nil
}

// rowScanner is the common interface of *sql.Row and *sql.Rows for
// the Scan method. Used by scanTrace/scanTraceFromRows to share the
// column-decoding logic.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanTrace reads a single traces row from a *sql.Row and decodes
// the JSON columns. The op parameter is threaded into error
// messages so Get vs GetBySessionTurn can be distinguished in
// logs. ErrTraceNotFound is returned if the row does not exist.
func scanTrace(row *sql.Row, op string) (FullManifest, error) {
	var (
		m         Manifest
		skillJSON string
		mcpJSON   string
		createdMs int64
	)
	err := row.Scan(
		&m.TraceID, &m.SessionID, &m.Turn, &m.PromptHash, &m.ToolSchemaHash,
		&skillJSON, &mcpJSON, &m.ModelID, &m.PromptEpoch, &createdMs,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FullManifest{}, fmt.Errorf("execmanifest: %s: %w", op, ErrTraceNotFound)
		}
		return FullManifest{}, fmt.Errorf("execmanifest: %s: scan: %w", op, err)
	}
	if err := decodeJSONArray(skillJSON, &m.SkillHashes); err != nil {
		return FullManifest{}, fmt.Errorf("execmanifest: %s: decode skill_hashes: %w", op, err)
	}
	if err := decodeJSONMap(mcpJSON, &m.McpVersions); err != nil {
		return FullManifest{}, fmt.Errorf("execmanifest: %s: decode mcp_versions: %w", op, err)
	}
	return FullManifest{
		Manifest:  m,
		CreatedAt: time.UnixMilli(createdMs),
	}, nil
}

// scanTraceFromRows is the *sql.Rows variant used by ListBySession.
// Separated from scanTrace because *sql.Rows.Scan signals
// "no rows" via rows.Next() returning false, not via a Scan error.
func scanTraceFromRows(rows *sql.Rows, op string) (FullManifest, error) {
	var (
		m         Manifest
		skillJSON string
		mcpJSON   string
		createdMs int64
	)
	err := rows.Scan(
		&m.TraceID, &m.SessionID, &m.Turn, &m.PromptHash, &m.ToolSchemaHash,
		&skillJSON, &mcpJSON, &m.ModelID, &m.PromptEpoch, &createdMs,
	)
	if err != nil {
		return FullManifest{}, fmt.Errorf("execmanifest: %s: scan: %w", op, err)
	}
	if err := decodeJSONArray(skillJSON, &m.SkillHashes); err != nil {
		return FullManifest{}, fmt.Errorf("execmanifest: %s: decode skill_hashes: %w", op, err)
	}
	if err := decodeJSONMap(mcpJSON, &m.McpVersions); err != nil {
		return FullManifest{}, fmt.Errorf("execmanifest: %s: decode mcp_versions: %w", op, err)
	}
	return FullManifest{
		Manifest:  m,
		CreatedAt: time.UnixMilli(createdMs),
	}, nil
}

// ErrCorruptJSON is returned when a JSON column contains an empty
// string, which the schema does not permit. A legitimate empty
// collection is stored as the JSON literal `[]` or `{}`, never `""`.
// An empty string indicates either data corruption or a schema
// violation from a non-public writer. Codex R10 caught that the
// previous "silently coerce empty to empty" behavior would mask a
// real corruption class.
var ErrCorruptJSON = errors.New("execmanifest: corrupt JSON column (empty string)")

// decodeJSONArray unmarshals into dst. An empty input is treated
// as a corruption signal (see ErrCorruptJSON), NOT as an empty
// collection — the canonicalJSONArray writer always emits `[]`
// for nil/empty, so reading back `""` means something has tampered
// with the row.
func decodeJSONArray(s string, dst *[]string) error {
	if s == "" {
		return fmt.Errorf("%w: skill_hashes expected []", ErrCorruptJSON)
	}
	return json.Unmarshal([]byte(s), dst)
}

// decodeJSONMap unmarshals into dst. Same empty-is-corruption
// treatment as decodeJSONArray.
func decodeJSONMap(s string, dst *map[string]string) error {
	if s == "" {
		return fmt.Errorf("%w: mcp_versions expected {}", ErrCorruptJSON)
	}
	return json.Unmarshal([]byte(s), dst)
}

// guardedRead runs the supplied closure under state.RLock() so
// Close is fenced against concurrent reads. The RLock is held for
// the full duration of fn, which means Close's state.Lock()
// acquisition blocks until every in-flight reader finishes — this
// is the intended "graceful shutdown waits for readers" semantics.
// Returns ErrClosed if the store is already closed when fn would
// start; otherwise propagates fn's error verbatim.
//
// Every public read method (Get, GetBySessionTurn, ListBySession)
// MUST go through guardedRead so a concurrent Close can never let
// a raw `sql: database is closed` error surface through the public
// API. Codex R10 caught this TOCTOU: prior iterations released the
// read-lock before starting the SQL, so a read could race Close
// between the isClosedLocked() check and the QueryRowContext call.
func (s *Store) guardedRead(fn func() error) error {
	s.state.RLock()
	defer s.state.RUnlock()
	if s.closedFlag {
		return ErrClosed
	}
	return fn()
}

// Close shuts down the writer goroutine and closes the database
// connection. Same shutdown sequence as pkg/actionlog:
//
//  1. state.Lock blocks until every in-flight enqueue releases.
//  2. closedFlag=true so future enqueues see ErrClosed.
//  3. close(s.closed) signals the writer actor to drain and exit.
//  4. Release state.Lock.
//  5. <-s.done waits for the writer to finish draining.
//  6. Close the underlying database.
//
// Close is idempotent; subsequent calls return nil.
func (s *Store) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	s.state.Lock()
	if s.closedFlag {
		s.state.Unlock()
		return nil
	}
	s.closedFlag = true
	close(s.closed)
	s.state.Unlock()

	<-s.done
	return s.db.Close()
}
