package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jiayaoqijia/ottie/pkg/swarm/board"
)

// ProjectBoardTool provides LLM access to the shared project board for multi-bot coordination.
type ProjectBoardTool struct {
	board      board.ProjectBoard
	instanceID string
}

// NewProjectBoardTool creates a new project board tool.
func NewProjectBoardTool(b board.ProjectBoard, instanceID string) *ProjectBoardTool {
	return &ProjectBoardTool{board: b, instanceID: instanceID}
}

func (t *ProjectBoardTool) Name() string { return "project_board" }

func (t *ProjectBoardTool) Description() string {
	return "Interact with the shared project board for multi-bot coordination. " +
		"Actions: read_tasks, post_task, claim_task, update_task, put_artifact, get_artifact, list_artifacts, put_context, get_context, handoff"
}

func (t *ProjectBoardTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum": []string{
					"read_tasks",
					"post_task",
					"claim_task",
					"update_task",
					"put_artifact",
					"get_artifact",
					"list_artifacts",
					"put_context",
					"get_context",
					"handoff",
				},
			},
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task ID (for claim_task, update_task)",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Task title (for post_task)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Task description (for post_task, update_task)",
			},
			"status": map[string]any{
				"type":        "string",
				"description": "Task status (for update_task): open, claimed, done, blocked",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "Key for artifact or context operations",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "Value for artifact or context operations",
			},
			"target_bot": map[string]any{
				"type":        "string",
				"description": "Target bot username for handoff",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "Message content for handoff",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ProjectBoardTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	if action == "" {
		return ErrorResult("action is required")
	}

	switch action {
	case "read_tasks":
		return t.readTasks(ctx)
	case "post_task":
		return t.postTask(ctx, args)
	case "claim_task":
		return t.claimTask(ctx, args)
	case "update_task":
		return t.updateTask(ctx, args)
	case "put_artifact":
		return t.putArtifact(ctx, args)
	case "get_artifact":
		return t.getArtifact(ctx, args)
	case "list_artifacts":
		return t.listArtifacts(ctx)
	case "put_context":
		return t.putContext(ctx, args)
	case "get_context":
		return t.getContext(ctx, args)
	case "handoff":
		return t.handoff(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *ProjectBoardTool) readTasks(ctx context.Context) *ToolResult {
	tasks, err := t.board.ListTasks(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list tasks: %v", err))
	}
	data, _ := json.Marshal(tasks)
	return &ToolResult{ForLLM: string(data)}
}

func (t *ProjectBoardTool) postTask(ctx context.Context, args map[string]any) *ToolResult {
	title, _ := args["title"].(string)
	if title == "" {
		return ErrorResult("title is required for post_task")
	}
	desc, _ := args["description"].(string)
	task := &board.BoardTask{
		Title:       title,
		Description: desc,
		CreatedBy:   t.instanceID,
	}
	if err := t.board.PostTask(ctx, task); err != nil {
		return ErrorResult(fmt.Sprintf("failed to post task: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Task created: %s (ID: %s)", title, task.ID)}
}

func (t *ProjectBoardTool) claimTask(ctx context.Context, args map[string]any) *ToolResult {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return ErrorResult("task_id is required for claim_task")
	}
	if err := t.board.ClaimTask(ctx, taskID, t.instanceID); err != nil {
		return ErrorResult(fmt.Sprintf("failed to claim task: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Task %s claimed by %s", taskID, t.instanceID)}
}

func (t *ProjectBoardTool) updateTask(ctx context.Context, args map[string]any) *ToolResult {
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return ErrorResult("task_id is required for update_task")
	}
	updates := make(map[string]string)
	for _, key := range []string{"status", "title", "description"} {
		if v, ok := args[key].(string); ok && v != "" {
			updates[key] = v
		}
	}
	if len(updates) == 0 {
		return ErrorResult("at least one of status, title, or description is required")
	}
	if err := t.board.UpdateTask(ctx, taskID, updates); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update task: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Task %s updated", taskID)}
}

func (t *ProjectBoardTool) putArtifact(ctx context.Context, args map[string]any) *ToolResult {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" || value == "" {
		return ErrorResult("key and value are required for put_artifact")
	}
	if err := t.board.PutArtifact(ctx, key, value, t.instanceID); err != nil {
		return ErrorResult(fmt.Sprintf("failed to put artifact: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Artifact '%s' stored", key)}
}

func (t *ProjectBoardTool) getArtifact(ctx context.Context, args map[string]any) *ToolResult {
	key, _ := args["key"].(string)
	if key == "" {
		return ErrorResult("key is required for get_artifact")
	}
	a, err := t.board.GetArtifact(ctx, key)
	if err != nil {
		return ErrorResult(fmt.Sprintf("artifact not found: %v", err))
	}
	data, _ := json.Marshal(a)
	return &ToolResult{ForLLM: string(data)}
}

func (t *ProjectBoardTool) listArtifacts(ctx context.Context) *ToolResult {
	artifacts, err := t.board.ListArtifacts(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list artifacts: %v", err))
	}
	data, _ := json.Marshal(artifacts)
	return &ToolResult{ForLLM: string(data)}
}

func (t *ProjectBoardTool) putContext(ctx context.Context, args map[string]any) *ToolResult {
	key, _ := args["key"].(string)
	value, _ := args["value"].(string)
	if key == "" {
		return ErrorResult("key is required for put_context")
	}
	if err := t.board.PutContext(ctx, key, value); err != nil {
		return ErrorResult(fmt.Sprintf("failed to put context: %v", err))
	}
	return &ToolResult{ForLLM: fmt.Sprintf("Context '%s' stored", key)}
}

func (t *ProjectBoardTool) getContext(ctx context.Context, args map[string]any) *ToolResult {
	key, _ := args["key"].(string)
	if key == "" {
		return ErrorResult("key is required for get_context")
	}
	v, err := t.board.GetContext(ctx, key)
	if err != nil {
		return ErrorResult(fmt.Sprintf("context key not found: %v", err))
	}
	return &ToolResult{ForLLM: v}
}

func (t *ProjectBoardTool) handoff(args map[string]any) *ToolResult {
	target, _ := args["target_bot"].(string)
	message, _ := args["message"].(string)
	if target == "" || message == "" {
		return ErrorResult("target_bot and message are required for handoff")
	}

	handoffBlock := fmt.Sprintf("```json\n{\"type\":\"handoff\",\"from\":\"%s\",\"to\":\"%s\",\"message\":\"%s\"}\n```",
		t.instanceID, target, strings.ReplaceAll(message, "\"", "\\\""))

	return &ToolResult{
		ForLLM:  fmt.Sprintf("Handoff message prepared for %s", target),
		ForUser: fmt.Sprintf("%s %s\n\n%s", target, message, handoffBlock),
	}
}
