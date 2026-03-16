package board

import (
	"context"
	"sync"
	"testing"
)

func TestMemoryBoard_Artifacts(t *testing.T) {
	b := NewMemoryBoard()
	ctx := context.Background()

	if err := b.PutArtifact(ctx, "design-doc", "## Architecture\nSwarm design", "agent-1"); err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}

	a, err := b.GetArtifact(ctx, "design-doc")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}

	if a.Key != "design-doc" || a.Value != "## Architecture\nSwarm design" || a.CreatedBy != "agent-1" {
		t.Errorf("unexpected artifact: %+v", a)
	}

	if a.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// List
	err = b.PutArtifact(ctx, "spec", "spec content", "agent-2")
	if err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}

	list, err := b.ListArtifacts(ctx)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(list))
	}

	// Not found
	_, err = b.GetArtifact(ctx, "missing")
	if err == nil {
		t.Error("expected error for missing artifact")
	}
}

func TestMemoryBoard_Tasks(t *testing.T) {
	b := NewMemoryBoard()
	ctx := context.Background()

	// Post with auto-ID
	task := &BoardTask{Title: "Implement feature X", CreatedBy: "agent-1"}
	if err := b.PostTask(ctx, task); err != nil {
		t.Fatalf("PostTask: %v", err)
	}

	if task.ID != "task-1" {
		t.Errorf("expected auto-assigned ID task-1, got %s", task.ID)
	}

	if task.Status != "open" {
		t.Errorf("expected default status open, got %s", task.Status)
	}

	if task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Error("timestamps should be set")
	}

	// Post with explicit ID
	task2 := &BoardTask{ID: "custom-id", Title: "Custom task", Status: "blocked"}
	if err := b.PostTask(ctx, task2); err != nil {
		t.Fatalf("PostTask: %v", err)
	}

	if task2.ID != "custom-id" || task2.Status != "blocked" {
		t.Errorf("unexpected task: %+v", task2)
	}

	// Get
	got, err := b.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if got.Title != "Implement feature X" {
		t.Errorf("unexpected title: %s", got.Title)
	}

	// Claim
	err = b.ClaimTask(ctx, "task-1", "agent-2")
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	got, _ = b.GetTask(ctx, "task-1")
	if got.Status != "claimed" || got.AssignedTo != "agent-2" {
		t.Errorf("expected claimed by agent-2, got status=%s assigned=%s", got.Status, got.AssignedTo)
	}

	// Update
	err = b.UpdateTask(ctx, "task-1", map[string]string{"status": "done", "title": "Updated title"})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	got, _ = b.GetTask(ctx, "task-1")
	if got.Status != "done" || got.Title != "Updated title" {
		t.Errorf("unexpected after update: %+v", got)
	}

	// List
	list, err := b.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	if len(list) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(list))
	}
}

func TestMemoryBoard_Context(t *testing.T) {
	b := NewMemoryBoard()
	ctx := context.Background()

	if err := b.PutContext(ctx, "goal", "build swarm feature"); err != nil {
		t.Fatalf("PutContext: %v", err)
	}

	v, err := b.GetContext(ctx, "goal")
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}

	if v != "build swarm feature" {
		t.Errorf("unexpected value: %s", v)
	}

	// Overwrite
	err = b.PutContext(ctx, "goal", "updated goal")
	if err != nil {
		t.Fatalf("PutContext: %v", err)
	}

	v, _ = b.GetContext(ctx, "goal")
	if v != "updated goal" {
		t.Errorf("expected updated goal, got %s", v)
	}

	// Not found
	_, err = b.GetContext(ctx, "missing")
	if err == nil {
		t.Error("expected error for missing context key")
	}
}

func TestMemoryBoard_TaskNotFound(t *testing.T) {
	b := NewMemoryBoard()
	ctx := context.Background()

	_, err := b.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for GetTask with nonexistent ID")
	}

	err = b.ClaimTask(ctx, "nonexistent", "agent-1")
	if err == nil {
		t.Error("expected error for ClaimTask with nonexistent ID")
	}

	err = b.UpdateTask(ctx, "nonexistent", map[string]string{"status": "done"})
	if err == nil {
		t.Error("expected error for UpdateTask with nonexistent ID")
	}
}

func TestMemoryBoard_ConcurrentAccess(t *testing.T) {
	b := NewMemoryBoard()
	ctx := context.Background()

	var wg sync.WaitGroup

	// Concurrent task posting
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			task := &BoardTask{Title: "concurrent task"}
			if err := b.PostTask(ctx, task); err != nil {
				t.Errorf("concurrent PostTask %d: %v", n, err)
			}
		}(i)
	}

	// Concurrent artifact writes
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			key := "key"
			if err := b.PutArtifact(ctx, key, "value", "agent"); err != nil {
				t.Errorf("concurrent PutArtifact %d: %v", n, err)
			}
		}(i)
	}

	// Concurrent context writes
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			if err := b.PutContext(ctx, "shared", "value"); err != nil {
				t.Errorf("concurrent PutContext %d: %v", n, err)
			}
		}(i)
	}

	// Concurrent reads
	for i := range 20 {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			_, _ = b.ListTasks(ctx)
			_, _ = b.ListArtifacts(ctx)
		}(i)
	}

	wg.Wait()

	tasks, err := b.ListTasks(ctx)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	if len(tasks) != 20 {
		t.Errorf("expected 20 tasks, got %d", len(tasks))
	}
}
