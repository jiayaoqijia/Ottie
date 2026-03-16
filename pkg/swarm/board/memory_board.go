package board

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Compile-time check that MemoryBoard implements ProjectBoard.
var _ ProjectBoard = (*MemoryBoard)(nil)

// MemoryBoard is an in-memory implementation of ProjectBoard.
type MemoryBoard struct {
	mu        sync.RWMutex
	tasks     map[string]*BoardTask
	artifacts map[string]*Artifact
	context   map[string]string
	nextID    int
}

// NewMemoryBoard creates a new in-memory project board.
func NewMemoryBoard() *MemoryBoard {
	return &MemoryBoard{
		tasks:     make(map[string]*BoardTask),
		artifacts: make(map[string]*Artifact),
		context:   make(map[string]string),
	}
}

func (m *MemoryBoard) PutArtifact(_ context.Context, key, value, createdBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.artifacts[key] = &Artifact{
		Key:       key,
		Value:     value,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}

	return nil
}

func (m *MemoryBoard) GetArtifact(_ context.Context, key string) (*Artifact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.artifacts[key]
	if !ok {
		return nil, fmt.Errorf("artifact %q not found", key)
	}

	return a, nil
}

func (m *MemoryBoard) ListArtifacts(_ context.Context) ([]*Artifact, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Artifact, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		result = append(result, a)
	}

	return result, nil
}

func (m *MemoryBoard) PostTask(_ context.Context, task *BoardTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if task.ID == "" {
		m.nextID++
		task.ID = fmt.Sprintf("task-%d", m.nextID)
	}

	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	if task.Status == "" {
		task.Status = "open"
	}

	m.tasks[task.ID] = task

	return nil
}

func (m *MemoryBoard) ClaimTask(_ context.Context, taskID, assignee string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	t.Status = "claimed"
	t.AssignedTo = assignee
	t.UpdatedAt = time.Now()

	return nil
}

func (m *MemoryBoard) UpdateTask(_ context.Context, taskID string, updates map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	for k, v := range updates {
		switch k {
		case "status":
			t.Status = v
		case "title":
			t.Title = v
		case "description":
			t.Description = v
		case "assigned_to":
			t.AssignedTo = v
		}
	}

	t.UpdatedAt = time.Now()

	return nil
}

func (m *MemoryBoard) ListTasks(_ context.Context) ([]*BoardTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*BoardTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, t)
	}

	return result, nil
}

func (m *MemoryBoard) GetTask(_ context.Context, taskID string) (*BoardTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	return t, nil
}

func (m *MemoryBoard) PutContext(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.context[key] = value

	return nil
}

func (m *MemoryBoard) GetContext(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.context[key]
	if !ok {
		return "", fmt.Errorf("context key %q not found", key)
	}

	return v, nil
}

func (m *MemoryBoard) Close() error {
	return nil
}
