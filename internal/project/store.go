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

func (store Store) LoadPartial() (Config, bool, error) {
	data, err := os.ReadFile(store.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read %s: %w", ConfigFileName, err)
	}

	config, err := ParsePartialConfig(data)
	if err != nil {
		return Config{}, true, fmt.Errorf("parse %s: %w", ConfigFileName, err)
	}
	return config, true, nil
}

func (store Store) WriteDefault() error {
	if err := os.WriteFile(store.ConfigPath(), []byte(DefaultConfigYAML()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ConfigFileName, err)
	}
	return nil
}

func (store Store) SavePartial(config Config) error {
	if config.Version == 0 {
		config.Version = SupportedConfigVersion
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

func (store Store) SelectWarehouse(warehouseType, defaultAuthName string) (Config, error) {
	if warehouseType == "" {
		return Config{}, errors.New("warehouse type is required")
	}

	config, exists, err := store.LoadPartial()
	if err != nil {
		return Config{}, err
	}
	if !exists {
		config = Config{Version: SupportedConfigVersion}
	}
	if config.Version == 0 {
		config.Version = SupportedConfigVersion
	}
	if config.Version != SupportedConfigVersion {
		return Config{}, fmt.Errorf("unsupported version %d; this CLI supports version %d", config.Version, SupportedConfigVersion)
	}
	if config.Warehouse.Type != "" && config.Warehouse.Type != warehouseType {
		return Config{}, fmt.Errorf("segmentstream.yml already uses warehouse.type %q", config.Warehouse.Type)
	}

	dirty := !exists
	if config.Warehouse.Type != warehouseType {
		config.Warehouse.Type = warehouseType
		dirty = true
	}
	if config.Warehouse.Auth == "" {
		config.Warehouse.Auth = defaultAuthName
		dirty = true
	}
	if dirty {
		if err := store.SavePartial(config); err != nil {
			return Config{}, err
		}
	}
	return config, nil
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
