package internal

import (
	"os"
	"path/filepath"

	"github.com/jiayaoqijia/ottie/pkg/config"
)

const Logo = "🦦"

// GetOttieHome returns the ottie home directory.
// Priority: $OTTIE_HOME > ~/.ottie
func GetOttieHome() string {
	if home := os.Getenv("OTTIE_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ottie")
}

func GetConfigPath() string {
	if configPath := os.Getenv("OTTIE_CONFIG"); configPath != "" {
		return configPath
	}
	return filepath.Join(GetOttieHome(), "config.json")
}

func LoadConfig() (*config.Config, error) {
	return config.LoadConfig(GetConfigPath())
}

// FormatVersion returns the version string with optional git commit
// Deprecated: Use pkg/config.FormatVersion instead
func FormatVersion() string {
	return config.FormatVersion()
}

// FormatBuildInfo returns build time and go version info
// Deprecated: Use pkg/config.FormatBuildInfo instead
func FormatBuildInfo() (string, string) {
	return config.FormatBuildInfo()
}

// GetVersion returns the version string
// Deprecated: Use pkg/config.GetVersion instead
func GetVersion() string {
	return config.GetVersion()
}
