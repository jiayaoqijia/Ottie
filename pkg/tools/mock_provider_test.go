package tools

import (
	"context"

	"github.com/jiayaoqijia/ottie/pkg/providers"
)

// MockLLMProvider is a shared test implementation of providers.LLMProvider
// used by spawn_test.go, swarm_test.go, and other tests in pkg/tools.
type MockLLMProvider struct {
	lastOptions map[string]any
}

func (m *MockLLMProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	m.lastOptions = options
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return &providers.LLMResponse{
				Content: "Task completed: " + messages[i].Content,
			}, nil
		}
	}
	return &providers.LLMResponse{Content: "No task provided"}, nil
}

func (m *MockLLMProvider) GetDefaultModel() string  { return "test-model" }
func (m *MockLLMProvider) SupportsTools() bool      { return false }
func (m *MockLLMProvider) GetContextWindow() int    { return 4096 }
