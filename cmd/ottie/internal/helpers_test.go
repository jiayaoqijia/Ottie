package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/tmp/home", ".ottie", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithOTTIE_HOME(t *testing.T) {
	t.Setenv("OTTIE_HOME", "/custom/ottie")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/custom/ottie", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithOTTIE_CONFIG(t *testing.T) {
	t.Setenv("OTTIE_CONFIG", "/custom/config.json")
	t.Setenv("OTTIE_HOME", "/custom/ottie")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := "/custom/config.json"

	assert.Equal(t, want, got)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	testUserProfilePath := `C:\Users\Test`
	t.Setenv("USERPROFILE", testUserProfilePath)

	got := GetConfigPath()
	want := filepath.Join(testUserProfilePath, ".ottie", "config.json")

	require.True(t, strings.EqualFold(got, want), "GetConfigPath() = %q, want %q", got, want)
}
