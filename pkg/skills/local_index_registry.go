package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// IndexEntry represents a single skill in the local index.
type IndexEntry struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Summary     string `json:"summary"`
	Version     string `json:"version"`
}

// LocalIndexConfig configures the local index registry.
type LocalIndexConfig struct {
	Enabled   bool
	IndexPath string
	Fallback  SkillRegistry // used for DownloadAndInstall delegation
}

// LocalIndexRegistry implements SkillRegistry using a local JSON index file.
// Searches are instant substring matches against the in-memory entries.
// Downloads are delegated to the fallback registry (typically ClawHub).
type LocalIndexRegistry struct {
	indexPath string
	fallback  SkillRegistry
	entries   []IndexEntry
	mu        sync.RWMutex
}

// NewLocalIndexRegistry creates a LocalIndexRegistry from config.
func NewLocalIndexRegistry(cfg LocalIndexConfig) *LocalIndexRegistry {
	indexPath := cfg.IndexPath
	if indexPath == "" {
		home, _ := os.UserHomeDir()
		indexPath = filepath.Join(home, ".ottie", "skills-index.json")
	}
	return &LocalIndexRegistry{
		indexPath: indexPath,
		fallback:  cfg.Fallback,
	}
}

func (r *LocalIndexRegistry) Name() string { return "local" }

// Load reads the JSON index file into memory. If the file doesn't exist,
// the registry starts with an empty index (no error).
func (r *LocalIndexRegistry) Load() error {
	data, err := os.ReadFile(r.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("local skill index not found, starting empty", "path", r.indexPath)
			return nil
		}
		return fmt.Errorf("failed to read index file: %w", err)
	}

	var entries []IndexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse index file: %w", err)
	}

	r.mu.Lock()
	r.entries = entries
	r.mu.Unlock()

	slog.Info("loaded local skill index", "count", len(entries), "path", r.indexPath)
	return nil
}

// Save writes the in-memory entries to the JSON index file.
func (r *LocalIndexRegistry) Save() error {
	r.mu.RLock()
	entries := make([]IndexEntry, len(r.entries))
	copy(entries, r.entries)
	r.mu.RUnlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	dir := filepath.Dir(r.indexPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}

	if err := os.WriteFile(r.indexPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

// SetEntries replaces the in-memory index (used by Sync and tests).
func (r *LocalIndexRegistry) SetEntries(entries []IndexEntry) {
	r.mu.Lock()
	r.entries = entries
	r.mu.Unlock()
}

// Entries returns a copy of the current in-memory index.
func (r *LocalIndexRegistry) Entries() []IndexEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]IndexEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Search performs case-insensitive substring matching against slug,
// display_name, and summary. Score is based on how many fields match.
func (r *LocalIndexRegistry) Search(_ context.Context, query string, limit int) ([]SearchResult, error) {
	r.mu.RLock()
	entries := r.entries
	r.mu.RUnlock()

	if len(entries) == 0 {
		return nil, nil
	}

	q := strings.ToLower(query)
	var results []SearchResult

	for _, e := range entries {
		var score float64

		slugLower := strings.ToLower(e.Slug)
		displayLower := strings.ToLower(e.DisplayName)
		summaryLower := strings.ToLower(e.Summary)

		if strings.Contains(slugLower, q) {
			score += 0.5
		}
		if strings.Contains(displayLower, q) {
			score += 0.3
		}
		if strings.Contains(summaryLower, q) {
			score += 0.2
		}

		if score > 0 {
			results = append(results, SearchResult{
				Score:        score,
				Slug:         e.Slug,
				DisplayName:  e.DisplayName,
				Summary:      e.Summary,
				Version:      e.Version,
				RegistryName: r.Name(),
			})
		}
	}

	// Sort by score descending.
	sortByScoreDesc(results)

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetSkillMeta looks up a skill by slug from the in-memory index.
func (r *LocalIndexRegistry) GetSkillMeta(_ context.Context, slug string) (*SkillMeta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, e := range r.entries {
		if e.Slug == slug {
			return &SkillMeta{
				Slug:          e.Slug,
				DisplayName:   e.DisplayName,
				Summary:       e.Summary,
				LatestVersion: e.Version,
				RegistryName:  r.Name(),
			}, nil
		}
	}

	return nil, fmt.Errorf("skill %q not found in local index", slug)
}

// DownloadAndInstall delegates to the fallback registry. The local index
// is search-only; actual downloads require a network registry.
func (r *LocalIndexRegistry) DownloadAndInstall(
	ctx context.Context, slug, version, targetDir string,
) (*InstallResult, error) {
	if r.fallback == nil {
		return nil, fmt.Errorf("local index is search-only: no fallback registry configured for downloads")
	}
	return r.fallback.DownloadAndInstall(ctx, slug, version, targetDir)
}

// Sync fetches the full skill catalog from a ClawHub-compatible registry
// and replaces the local index. The registry must implement the Search
// interface — we paginate by requesting large batches.
func (r *LocalIndexRegistry) Sync(ctx context.Context, source SkillRegistry) (int, error) {
	if source == nil {
		return 0, fmt.Errorf("no source registry provided for sync")
	}

	// Fetch a large batch — ClawHub search with empty query returns all.
	results, err := source.Search(ctx, "", 10000)
	if err != nil {
		return 0, fmt.Errorf("sync fetch failed: %w", err)
	}

	entries := make([]IndexEntry, 0, len(results))
	for _, sr := range results {
		entries = append(entries, IndexEntry{
			Slug:        sr.Slug,
			DisplayName: sr.DisplayName,
			Summary:     sr.Summary,
			Version:     sr.Version,
		})
	}

	r.SetEntries(entries)
	if err := r.Save(); err != nil {
		return len(entries), fmt.Errorf("sync succeeded but save failed: %w", err)
	}

	slog.Info("synced local skill index", "count", len(entries))
	return len(entries), nil
}
