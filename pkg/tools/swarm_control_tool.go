package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// SessionsControlTool controls running sub-agent sessions via SwarmManager.
// Only registered for orchestrator agents.
type SessionsControlTool struct {
	manager *SwarmManager
}

// NewSessionsControlTool creates a new sessions_control tool.
func NewSessionsControlTool(manager *SwarmManager) *SessionsControlTool {
	return &SessionsControlTool{manager: manager}
}

func (t *SessionsControlTool) Name() string { return "sessions_control" }

func (t *SessionsControlTool) Description() string {
	return "Control running sub-agent sessions: list active runs, kill a run, steer with new instructions, or get run info."
}

func (t *SessionsControlTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"list", "kill", "steer", "info"},
			},
			"run_id": map[string]any{
				"type":        "string",
				"description": "Run ID (required for kill, steer, info)",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message to send (for steer action)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SessionsControlTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required")
	}

	switch action {
	case "list":
		return t.listRuns()
	case "kill":
		return t.killRun(args)
	case "steer":
		return t.steerRun(args)
	case "info":
		return t.runInfo(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

type runSummary struct {
	RunID   string `json:"run_id"`
	AgentID string `json:"agent_id,omitempty"`
	Label   string `json:"label,omitempty"`
	Status  string `json:"status"`
	Depth   int    `json:"depth"`
	Task    string `json:"task"`
}

func (t *SessionsControlTool) listRuns() *ToolResult {
	runs := t.manager.ListRuns("")
	summaries := make([]runSummary, 0, len(runs))
	for _, r := range runs {
		task := r.Task
		if len(task) > 100 {
			task = task[:100] + "..."
		}
		summaries = append(summaries, runSummary{
			RunID:   r.RunID,
			AgentID: r.AgentID,
			Label:   r.Label,
			Status:  r.Status,
			Depth:   r.Depth,
			Task:    task,
		})
	}
	data, _ := json.Marshal(summaries)
	return &ToolResult{ForLLM: string(data)}
}

func (t *SessionsControlTool) killRun(args map[string]any) *ToolResult {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return ErrorResult("run_id is required for kill action")
	}
	if err := t.manager.Kill(runID); err != nil {
		return ErrorResult(fmt.Sprintf("failed to kill run: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Run %s killed (and all children)", runID)}
}

func (t *SessionsControlTool) steerRun(args map[string]any) *ToolResult {
	runID, _ := args["run_id"].(string)
	message, _ := args["message"].(string)
	if runID == "" {
		return ErrorResult("run_id is required for steer action")
	}
	if message == "" {
		return ErrorResult("message is required for steer action")
	}
	if err := t.manager.Steer(runID, message); err != nil {
		return ErrorResult(fmt.Sprintf("failed to steer run: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Steer message sent to run %s", runID)}
}

func (t *SessionsControlTool) runInfo(args map[string]any) *ToolResult {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return ErrorResult("run_id is required for info action")
	}
	run, ok := t.manager.GetRun(runID)
	if !ok {
		return ErrorResult(fmt.Sprintf("run %q not found", runID))
	}

	info := map[string]any{
		"run_id":     run.RunID,
		"parent_id":  run.ParentRunID,
		"agent_id":   run.AgentID,
		"label":      run.Label,
		"status":     run.Status,
		"depth":      run.Depth,
		"task":       run.Task,
		"outcome":    run.Outcome,
		"created_at": run.CreatedAt,
	}
	if !run.CompletedAt.IsZero() {
		info["completed_at"] = run.CompletedAt
	}
	data, _ := json.Marshal(info)
	return &ToolResult{ForLLM: string(data)}
}
