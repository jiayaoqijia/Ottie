package session

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Archiver handles gzip-compressed session archival and restoration.
type Archiver struct {
	dir string
}

// NewArchiver creates an Archiver that stores compressed session files in dir.
// The directory is created with 0o700 permissions if it does not exist.
func NewArchiver(dir string) (*Archiver, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Archiver{dir: dir}, nil
}

// Archive writes a gzip-compressed JSON representation of the session to disk.
// It uses an atomic write pattern (temp file + rename) to avoid partial writes.
func (a *Archiver) Archive(session *Session) error {
	filename := sanitizeFilename(session.Key)
	if filename == "." || !filepath.IsLocal(filename) {
		return os.ErrInvalid
	}

	archivePath := filepath.Join(a.dir, filename+".json.gz")

	tmpFile, err := os.CreateTemp(a.dir, "archive-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	gw := gzip.NewWriter(tmpFile)

	if err := json.NewEncoder(gw).Encode(session); err != nil {
		_ = gw.Close()
		_ = tmpFile.Close()
		return err
	}

	if err := gw.Close(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Chmod(0o600); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		return err
	}

	cleanup = false
	return nil
}

// Restore reads a gzip-compressed JSON session file and returns the Session.
func (a *Archiver) Restore(key string) (*Session, error) {
	filename := sanitizeFilename(key)
	archivePath := filepath.Join(a.dir, filename+".json.gz")

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	var session Session
	if err := json.NewDecoder(gr).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

// List returns the session keys of all archived sessions by reading the
// archive directory and stripping the .json.gz suffix.
func (a *Archiver) List() ([]string, error) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json.gz") {
			keys = append(keys, strings.TrimSuffix(name, ".json.gz"))
		}
	}

	return keys, nil
}
