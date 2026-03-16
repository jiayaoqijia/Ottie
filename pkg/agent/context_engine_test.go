package agent

import (
	"testing"

	"github.com/jiayaoqijia/ottie/pkg/providers"
)

func TestDefaultContextEngine_Assemble(t *testing.T) {
	engine := NewDefaultContextEngine()

	t.Run("returns copy of history", func(t *testing.T) {
		history := []providers.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		}
		result, err := engine.Assemble("session-1", history, 4096)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != len(history) {
			t.Fatalf("expected %d messages, got %d", len(history), len(result))
		}
		for i, msg := range result {
			if msg.Role != history[i].Role || msg.Content != history[i].Content {
				t.Errorf("message %d mismatch: got %+v, want %+v", i, msg, history[i])
			}
		}
		// Verify it's a copy, not the same slice
		result[0].Content = "modified"
		if history[0].Content == "modified" {
			t.Error("Assemble should return a copy, not the original slice")
		}
	})

	t.Run("empty history returns empty slice", func(t *testing.T) {
		result, err := engine.Assemble("session-2", nil, 4096)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Fatalf("expected empty result, got %d messages", len(result))
		}
	})
}

func TestDefaultContextEngine_Compact(t *testing.T) {
	engine := NewDefaultContextEngine()

	t.Run("returns history unchanged", func(t *testing.T) {
		history := []providers.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "how are you"},
		}
		result, err := engine.Compact("session-1", history, 4096)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Messages) != len(history) {
			t.Fatalf("expected %d messages, got %d", len(history), len(result.Messages))
		}
		if result.Summary != "" {
			t.Errorf("expected empty summary, got %q", result.Summary)
		}
		if result.Pruned != 0 {
			t.Errorf("expected 0 pruned, got %d", result.Pruned)
		}
	})
}

func TestDefaultContextEngine_ImplementsInterface(t *testing.T) {
	var _ ContextEngine = (*DefaultContextEngine)(nil)
}
