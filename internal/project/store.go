package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Store struct {
	Root string
}

func (store Store) ConfigPath() string {
	return filepath.Join(store.Root, ConfigFileName)
}

func (store Store) Exists() (bool, error) {
	if _, err := os.Stat(store.ConfigPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check %s: %w", ConfigFileName, err)
	}
	return true, nil
}

func (store Store) Load() (Config, error) {
	data, err := os.ReadFile(store.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("%s was not found in the current directory; run segmentstream init first", ConfigFileName)
		}
		return Config{}, fmt.Errorf("read %s: %w", ConfigFileName, err)
	}

	config, err := ParseConfig(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", ConfigFileName, err)
	}
	return config, nil
}

func (store Store) WriteDefault() error {
	if err := os.WriteFile(store.ConfigPath(), []byte(DefaultConfigYAML()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ConfigFileName, err)
	}
	return nil
}

func (store Store) Save(config Config) error {
	if err := ValidateConfig(config); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ConfigFileName, err)
	}
	if err := os.WriteFile(store.ConfigPath(), data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ConfigFileName, err)
	}
	return nil
}
