package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLocalRegistry(t *testing.T, entries []IndexEntry) *LocalIndexRegistry {
	t.Helper()
	reg := &LocalIndexRegistry{
		indexPath: filepath.Join(t.TempDir(), "test-index.json"),
		entries:   entries,
	}
	return reg
}

func TestLocalIndexRegistryName(t *testing.T) {
	reg := newTestLocalRegistry(t, nil)
	assert.Equal(t, "local", reg.Name())
}

func TestLocalIndexRegistrySearchBasic(t *testing.T) {
	entries := []IndexEntry{
		{Slug: "docker-compose", DisplayName: "Docker Compose", Summary: "Manage multi-container Docker apps"},
		{Slug: "github-actions", DisplayName: "GitHub Actions", Summary: "CI/CD automation for GitHub"},
		{Slug: "kubernetes", DisplayName: "Kubernetes", Summary: "Container orchestration platform"},
	}
	reg := newTestLocalRegistry(t, entries)

	results, err := reg.Search(context.Background(), "docker", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "docker-compose", results[0].Slug)
	assert.Equal(t, "local", results[0].RegistryName)
	assert.Greater(t, results[0].Score, 0.0)
}

func TestLocalIndexRegistrySearchMultiFieldMatch(t *testing.T) {
	entries := []IndexEntry{
		{Slug: "forecast", DisplayName: "Weather Forecast", Summary: "Get current conditions"},
		{Slug: "stock-weather", DisplayName: "Stock Weather", Summary: "Financial weather report"},
	}
	reg := newTestLocalRegistry(t, entries)

	results, err := reg.Search(context.Background(), "weather", 10)
	require.NoError(t, err)
	require.Len(t, results, 2)
	// "stock-weather" matches slug+display+summary (1.0) vs "forecast" matches display only (0.3)
	assert.Equal(t, "stock-weather", results[0].Slug)
}

func TestLocalIndexRegistrySearchCaseInsensitive(t *testing.T) {
	entries := []IndexEntry{
		{Slug: "MySkill", DisplayName: "My Skill", Summary: "Does things"},
	}
	reg := newTestLocalRegistry(t, entries)

	results, err := reg.Search(context.Background(), "myskill", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestLocalIndexRegistrySearchNoMatch(t *testing.T) {
	entries := []IndexEntry{
		{Slug: "docker", DisplayName: "Docker", Summary: "Container runtime"},
	}
	reg := newTestLocalRegistry(t, entries)

	results, err := reg.Search(context.Background(), "kubernetes", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLocalIndexRegistrySearchEmptyIndex(t *testing.T) {
	reg := newTestLocalRegistry(t, nil)

	results, err := reg.Search(context.Background(), "anything", 10)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestLocalIndexRegistrySearchRespectLimit(t *testing.T) {
	entries := make([]IndexEntry, 20)
	for i := range entries {
		entries[i] = IndexEntry{
			Slug:    "skill-match",
			Summary: "matches",
		}
	}
	reg := newTestLocalRegistry(t, entries)

	results, err := reg.Search(context.Background(), "match", 5)
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

func TestLocalIndexRegistryGetSkillMeta(t *testing.T) {
	entries := []IndexEntry{
		{Slug: "docker", DisplayName: "Docker", Summary: "Containers", Version: "1.2.0"},
	}
	reg := newTestLocalRegistry(t, entries)

	meta, err := reg.GetSkillMeta(context.Background(), "docker")
	require.NoError(t, err)
	assert.Equal(t, "docker", meta.Slug)
	assert.Equal(t, "Docker", meta.DisplayName)
	assert.Equal(t, "1.2.0", meta.LatestVersion)
	assert.Equal(t, "local", meta.RegistryName)
}

func TestLocalIndexRegistryGetSkillMetaNotFound(t *testing.T) {
	reg := newTestLocalRegistry(t, nil)

	_, err := reg.GetSkillMeta(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestLocalIndexRegistryDownloadAndInstallNoFallback(t *testing.T) {
	reg := newTestLocalRegistry(t, nil)

	_, err := reg.DownloadAndInstall(context.Background(), "test", "", "/tmp/test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no fallback registry configured")
}

func TestLocalIndexRegistryDownloadAndInstallDelegates(t *testing.T) {
	mock := &mockRegistry{
		name: "clawhub",
		installResult: &InstallResult{
			Version: "1.0.0",
			Summary: "test skill",
		},
	}
	reg := newTestLocalRegistry(t, nil)
	reg.fallback = mock

	result, err := reg.DownloadAndInstall(context.Background(), "test-skill", "", "/tmp/test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", result.Version)
}

func TestLocalIndexRegistrySaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")

	reg := &LocalIndexRegistry{
		indexPath: indexPath,
		entries: []IndexEntry{
			{Slug: "skill-a", DisplayName: "Skill A", Summary: "First skill", Version: "1.0.0"},
			{Slug: "skill-b", DisplayName: "Skill B", Summary: "Second skill", Version: "2.0.0"},
		},
	}

	// Save
	err := reg.Save()
	require.NoError(t, err)

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	var parsed []IndexEntry
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Len(t, parsed, 2)

	// Load into new registry
	reg2 := &LocalIndexRegistry{indexPath: indexPath}
	err = reg2.Load()
	require.NoError(t, err)
	assert.Len(t, reg2.Entries(), 2)
	assert.Equal(t, "skill-a", reg2.Entries()[0].Slug)
}

func TestLocalIndexRegistryLoadMissingFile(t *testing.T) {
	reg := &LocalIndexRegistry{
		indexPath: filepath.Join(t.TempDir(), "nonexistent.json"),
	}
	err := reg.Load()
	assert.NoError(t, err) // missing file is not an error
	assert.Empty(t, reg.Entries())
}

func TestLocalIndexRegistryLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(indexPath, []byte("not json"), 0o644))

	reg := &LocalIndexRegistry{indexPath: indexPath}
	err := reg.Load()
	assert.Error(t, err)
}

func TestLocalIndexRegistrySync(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "synced.json")

	source := &mockRegistry{
		name: "clawhub",
		searchResults: []SearchResult{
			{Slug: "skill-x", DisplayName: "Skill X", Summary: "X does things", Version: "3.0.0"},
			{Slug: "skill-y", DisplayName: "Skill Y", Summary: "Y does stuff", Version: "1.5.0"},
		},
	}

	reg := &LocalIndexRegistry{indexPath: indexPath}
	count, err := reg.Sync(context.Background(), source)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Verify entries are in memory
	entries := reg.Entries()
	assert.Len(t, entries, 2)
	assert.Equal(t, "skill-x", entries[0].Slug)

	// Verify file was written
	_, err = os.Stat(indexPath)
	assert.NoError(t, err)
}

func TestLocalIndexRegistrySyncNoSource(t *testing.T) {
	reg := newTestLocalRegistry(t, nil)
	_, err := reg.Sync(context.Background(), nil)
	assert.Error(t, err)
}

func TestLocalIndexRegistryIntegrationWithRegistryManager(t *testing.T) {
	// Verify local index works as a registry inside RegistryManager
	entries := []IndexEntry{
		{Slug: "local-skill", DisplayName: "Local Skill", Summary: "From local index", Version: "1.0.0"},
	}
	localReg := newTestLocalRegistry(t, entries)

	remote := &mockRegistry{
		name: "clawhub",
		searchResults: []SearchResult{
			{
				Slug: "remote-skill", DisplayName: "Remote Skill",
				Summary: "From local clawhub", Score: 0.9, RegistryName: "clawhub",
			},
		},
	}

	mgr := NewRegistryManager()
	mgr.AddRegistry(localReg)
	mgr.AddRegistry(remote)

	results, err := mgr.SearchAll(context.Background(), "local", 10)
	require.NoError(t, err)
	// Should include results from both registries
	assert.GreaterOrEqual(t, len(results), 1)

	// Local registry should be findable
	assert.NotNil(t, mgr.GetRegistry("local"))
}
