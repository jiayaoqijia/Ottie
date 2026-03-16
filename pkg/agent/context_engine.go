package agent

import "github.com/jiayaoqijia/ottie/pkg/providers"

// ContextEngine assembles and compacts conversation context.
type ContextEngine interface {
	Assemble(sessionKey string, history []providers.Message, tokenBudget int) ([]providers.Message, error)
	Compact(sessionKey string, history []providers.Message, tokenBudget int) (*CompactResult, error)
}

// CompactResult holds the output of a context compaction.
type CompactResult struct {
	Messages []providers.Message
	Summary  string
	Pruned   int
}

// DefaultContextEngine is a pass-through implementation that wraps
// existing summarization logic. No behavior change — just a cleaner interface.
type DefaultContextEngine struct{}

// NewDefaultContextEngine creates a new DefaultContextEngine.
func NewDefaultContextEngine() *DefaultContextEngine {
	return &DefaultContextEngine{}
}

// Assemble returns history as-is (no transformation).
func (e *DefaultContextEngine) Assemble(_ string, history []providers.Message, _ int) ([]providers.Message, error) {
	result := make([]providers.Message, len(history))
	copy(result, history)
	return result, nil
}

// Compact is a no-op placeholder. The actual summarization logic remains in loop.go
// and will be migrated in a future refactor.
func (e *DefaultContextEngine) Compact(_ string, history []providers.Message, _ int) (*CompactResult, error) {
	return &CompactResult{
		Messages: history,
		Summary:  "",
		Pruned:   0,
	}, nil
}
