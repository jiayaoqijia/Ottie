package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jiayaoqijia/ottie/pkg/swarm/board"
)

func newTestBoardTool() (*ProjectBoardTool, board.ProjectBoard) {
	b := board.NewMemoryBoard()
	tool := NewProjectBoardTool(b, "test-bot")
	return tool, b
}

func TestProjectBoardTool_ReadTasks(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Empty board
	result := tool.Execute(ctx, map[string]any{"action": "read_tasks"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM != "[]" {
		t.Errorf("expected empty array, got %s", result.ForLLM)
	}

	// Post a task then read
	tool.Execute(ctx, map[string]any{"action": "post_task", "title": "Test task"})
	result = tool.Execute(ctx, map[string]any{"action": "read_tasks"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	var tasks []*board.BoardTask
	if err := json.Unmarshal([]byte(result.ForLLM), &tasks); err != nil {
		t.Fatalf("failed to unmarshal tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Test task" {
		t.Errorf("expected title 'Test task', got '%s'", tasks[0].Title)
	}
}

func TestProjectBoardTool_PostTask(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Success
	result := tool.Execute(
		ctx,
		map[string]any{"action": "post_task", "title": "My task", "description": "Do something"},
	)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Task created: My task") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}

	// Missing title
	result = tool.Execute(ctx, map[string]any{"action": "post_task"})
	if !result.IsError {
		t.Error("expected error for missing title")
	}
	if !strings.Contains(result.ForLLM, "title is required") {
		t.Errorf("unexpected error message: %s", result.ForLLM)
	}
}

func TestProjectBoardTool_ClaimTask(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Post a task first
	tool.Execute(ctx, map[string]any{"action": "post_task", "title": "Claimable"})

	// Read to get the task ID
	readResult := tool.Execute(ctx, map[string]any{"action": "read_tasks"})
	var tasks []*board.BoardTask
	json.Unmarshal([]byte(readResult.ForLLM), &tasks)
	taskID := tasks[0].ID

	// Claim it
	result := tool.Execute(ctx, map[string]any{"action": "claim_task", "task_id": taskID})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "claimed by test-bot") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}

	// Missing task_id
	result = tool.Execute(ctx, map[string]any{"action": "claim_task"})
	if !result.IsError {
		t.Error("expected error for missing task_id")
	}
}

func TestProjectBoardTool_UpdateTask(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Post a task
	tool.Execute(ctx, map[string]any{"action": "post_task", "title": "Updatable"})
	readResult := tool.Execute(ctx, map[string]any{"action": "read_tasks"})
	var tasks []*board.BoardTask
	json.Unmarshal([]byte(readResult.ForLLM), &tasks)
	taskID := tasks[0].ID

	// Update status
	result := tool.Execute(ctx, map[string]any{"action": "update_task", "task_id": taskID, "status": "done"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "updated") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}

	// Missing task_id
	result = tool.Execute(ctx, map[string]any{"action": "update_task", "status": "done"})
	if !result.IsError {
		t.Error("expected error for missing task_id")
	}

	// No updates provided
	result = tool.Execute(ctx, map[string]any{"action": "update_task", "task_id": taskID})
	if !result.IsError {
		t.Error("expected error for no updates")
	}
}

func TestProjectBoardTool_Artifacts(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Put artifact
	result := tool.Execute(ctx, map[string]any{"action": "put_artifact", "key": "design-doc", "value": "contents here"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "stored") {
		t.Errorf("unexpected result: %s", result.ForLLM)
	}

	// Get artifact
	result = tool.Execute(ctx, map[string]any{"action": "get_artifact", "key": "design-doc"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	var artifact board.Artifact
	if err := json.Unmarshal([]byte(result.ForLLM), &artifact); err != nil {
		t.Fatalf("failed to unmarshal artifact: %v", err)
	}
	if artifact.Value != "contents here" {
		t.Errorf("expected value 'contents here', got '%s'", artifact.Value)
	}

	// List artifacts
	result = tool.Execute(ctx, map[string]any{"action": "list_artifacts"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	var artifacts []*board.Artifact
	json.Unmarshal([]byte(result.ForLLM), &artifacts)
	if len(artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(artifacts))
	}

	// Missing key for get
	result = tool.Execute(ctx, map[string]any{"action": "get_artifact"})
	if !result.IsError {
		t.Error("expected error for missing key")
	}

	// Missing key/value for put
	result = tool.Execute(ctx, map[string]any{"action": "put_artifact", "key": "k"})
	if !result.IsError {
		t.Error("expected error for missing value")
	}
}

func TestProjectBoardTool_Context(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Put context
	result := tool.Execute(ctx, map[string]any{"action": "put_context", "key": "branch", "value": "feature-x"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}

	// Get context
	result = tool.Execute(ctx, map[string]any{"action": "get_context", "key": "branch"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM != "feature-x" {
		t.Errorf("expected 'feature-x', got '%s'", result.ForLLM)
	}

	// Missing key for get
	result = tool.Execute(ctx, map[string]any{"action": "get_context"})
	if !result.IsError {
		t.Error("expected error for missing key")
	}

	// Missing key for put
	result = tool.Execute(ctx, map[string]any{"action": "put_context"})
	if !result.IsError {
		t.Error("expected error for missing key")
	}
}

func TestProjectBoardTool_Handoff(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	// Success
	result := tool.Execute(
		ctx,
		map[string]any{"action": "handoff", "target_bot": "@coder_bot", "message": "Please review PR #42"},
	)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Handoff message prepared for @coder_bot") {
		t.Errorf("unexpected ForLLM: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForUser, "@coder_bot") {
		t.Errorf("expected ForUser to contain target bot: %s", result.ForUser)
	}
	if !strings.Contains(result.ForUser, "handoff") {
		t.Errorf("expected ForUser to contain handoff block: %s", result.ForUser)
	}

	// Missing params
	result = tool.Execute(ctx, map[string]any{"action": "handoff", "target_bot": "@bot"})
	if !result.IsError {
		t.Error("expected error for missing message")
	}
	result = tool.Execute(ctx, map[string]any{"action": "handoff", "message": "hi"})
	if !result.IsError {
		t.Error("expected error for missing target_bot")
	}
}

func TestProjectBoardTool_UnknownAction(t *testing.T) {
	tool, _ := newTestBoardTool()
	ctx := context.Background()

	result := tool.Execute(ctx, map[string]any{"action": "invalid_action"})
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
	if !strings.Contains(result.ForLLM, "unknown action") {
		t.Errorf("unexpected error message: %s", result.ForLLM)
	}

	// Missing action
	result = tool.Execute(ctx, map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing action")
	}
}
