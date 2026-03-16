package session

import (
	"testing"
	"time"

	"github.com/jiayaoqijia/ottie/pkg/providers"
)

func TestArchiverNewCreatesDir(t *testing.T) {
	dir := t.TempDir() + "/archives"
	a, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil Archiver")
	}
}

func TestArchiverRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	session := &Session{
		Key: "test:session/1",
		Messages: []providers.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
		Summary: "greeting",
		Created: now,
		Updated: now,
	}

	err = a.Archive(session)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}

	restored, err := a.Restore("test:session/1")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if restored.Key != session.Key {
		t.Errorf("key: got %q, want %q", restored.Key, session.Key)
	}
	if restored.Summary != session.Summary {
		t.Errorf("summary: got %q, want %q", restored.Summary, session.Summary)
	}
	if len(restored.Messages) != len(session.Messages) {
		t.Fatalf("messages count: got %d, want %d", len(restored.Messages), len(session.Messages))
	}
	for i, msg := range restored.Messages {
		if msg.Role != session.Messages[i].Role || msg.Content != session.Messages[i].Content {
			t.Errorf("message[%d]: got {%s, %s}, want {%s, %s}",
				i, msg.Role, msg.Content, session.Messages[i].Role, session.Messages[i].Content)
		}
	}
}

func TestArchiverList(t *testing.T) {
	dir := t.TempDir()
	a, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver: %v", err)
	}

	now := time.Now()
	sessions := []*Session{
		{Key: "session-a", Messages: []providers.Message{}, Created: now, Updated: now},
		{Key: "session-b", Messages: []providers.Message{}, Created: now, Updated: now},
	}

	for _, s := range sessions {
		archiveErr := a.Archive(s)
		if archiveErr != nil {
			t.Fatalf("Archive %s: %v", s.Key, archiveErr)
		}
	}

	keys, err := a.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("List count: got %d, want 2", len(keys))
	}
}

func TestArchiverRestoreNotFound(t *testing.T) {
	dir := t.TempDir()
	a, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver: %v", err)
	}

	_, err = a.Restore("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestArchiverInvalidKey(t *testing.T) {
	dir := t.TempDir()
	a, err := NewArchiver(dir)
	if err != nil {
		t.Fatalf("NewArchiver: %v", err)
	}

	session := &Session{Key: "..", Messages: []providers.Message{}}
	if err := a.Archive(session); err == nil {
		t.Fatal("expected error for invalid key '..'")
	}
}
