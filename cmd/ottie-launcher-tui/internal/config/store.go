package configstore

import (
	"errors"
	"os"
	"path/filepath"

	ottieconfig "github.com/jiayaoqijia/ottie/pkg/config"
)

const (
	configDirName  = ".ottie"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*ottieconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return ottieconfig.LoadConfig(path)
}

func Save(cfg *ottieconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return ottieconfig.SaveConfig(path, cfg)
}
