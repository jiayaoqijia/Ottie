package tools

import (
	"context"
	"fmt"
	"strings"
)

// SessionsSpawnTool spawns sub-agent sessions via SwarmManager.
// Only registered for orchestrator agents with subagents configured.
type SessionsSpawnTool struct {
	manager        *SwarmManager
	allowlistCheck func(targetAgentID string) bool
}

// Compile-time check: SessionsSpawnTool implements AsyncExecutor.
var _ AsyncExecutor = (*SessionsSpawnTool)(nil)

// NewSessionsSpawnTool creates a new sessions_spawn tool.
func NewSessionsSpawnTool(manager *SwarmManager) *SessionsSpawnTool {
	return &SessionsSpawnTool{manager: manager}
}

func (t *SessionsSpawnTool) Name() string { return "delegate" }

func (t *SessionsSpawnTool) Description() string {
	return "Delegate a task to a specialized sub-agent session. The sub-agent runs independently and reports back when done."
}

func (t *SessionsSpawnTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The task for the sub-agent to complete",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Short label for tracking (e.g. 'research-api')",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID to delegate to (must be in allow list)",
			},
		},
		"required": []string{"task"},
	}
}

// SetAllowlistChecker sets the function to check if a target agent can be spawned.
func (t *SessionsSpawnTool) SetAllowlistChecker(check func(targetAgentID string) bool) {
	t.allowlistCheck = check
}

func (t *SessionsSpawnTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	return t.execute(ctx, args, nil)
}

// ExecuteAsync implements AsyncExecutor.
func (t *SessionsSpawnTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	return t.execute(ctx, args, cb)
}

func (t *SessionsSpawnTool) execute(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	task, ok := args["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return ErrorResult("task is required and must be a non-empty string")
	}

	label, _ := args["label"].(string)
	agentID, _ := args["agent_id"].(string)

	// Check allowlist if targeting a specific agent
	if agentID != "" && t.allowlistCheck != nil {
		if !t.allowlistCheck(agentID) {
			return ErrorResult(fmt.Sprintf("not allowed to spawn agent '%s'", agentID))
		}
	}

	if t.manager == nil {
		return ErrorResult("swarm manager not configured")
	}

	channel := ToolChannel(ctx)
	if channel == "" {
		channel = "cli"
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = "direct"
	}

	result, err := t.manager.Spawn(ctx, SpawnParams{
		Task:          task,
		Label:         label,
		AgentID:       agentID,
		OriginChannel: channel,
		OriginChatID:  chatID,
		Callback:      cb,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to spawn sub-agent: %v", err))
	}

	return AsyncResult(result)
}
