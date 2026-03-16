package board

import (
	"context"
	"time"
)

// BoardTask represents a task on the shared project board.
type BoardTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"` // open, claimed, done, blocked
	AssignedTo  string    `json:"assigned_to,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	Artifacts   []string  `json:"artifacts,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Artifact represents a shared artifact on the project board.
type Artifact struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ProjectBoard defines the interface for multi-bot coordination.
// Implementations include in-memory (for testing/single-process) and Redis (for multi-process).
type ProjectBoard interface { //nolint:interfacebloat // coordination board needs all methods
	// Artifacts
	PutArtifact(ctx context.Context, key, value, createdBy string) error
	GetArtifact(ctx context.Context, key string) (*Artifact, error)
	ListArtifacts(ctx context.Context) ([]*Artifact, error)

	// Tasks
	PostTask(ctx context.Context, task *BoardTask) error
	ClaimTask(ctx context.Context, taskID, assignee string) error
	UpdateTask(ctx context.Context, taskID string, updates map[string]string) error
	ListTasks(ctx context.Context) ([]*BoardTask, error)
	GetTask(ctx context.Context, taskID string) (*BoardTask, error)

	// Context (shared key-value store for coordination)
	PutContext(ctx context.Context, key, value string) error
	GetContext(ctx context.Context, key string) (string, error)

	// Lifecycle
	Close() error
}
