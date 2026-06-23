package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	segmentStreamDirName = ".segmentstream"
	accessMarkerSuffix   = ".access.json"
)

type Store struct {
	HomeDir string
}

func (store Store) CredentialPath(warehouseType, name string) (string, error) {
	if err := validateWarehouseType(warehouseType); err != nil {
		return "", err
	}
	if err := validateCredentialName(name); err != nil {
		return "", err
	}
	root, err := store.warehouseDir(warehouseType)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name+".json"), nil
}

func (store Store) HasCredential(warehouseType, name string) (bool, error) {
	path, err := store.CredentialPath(warehouseType, name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check %s credential %q: %w", warehouseType, name, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("%s credential path %s is a directory", warehouseType, path)
	}
	return true, nil
}

func (store Store) SaveCredentialData(warehouseType, name string, data []byte) (string, error) {
	path, err := store.CredentialPath(warehouseType, name)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("credential data is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create %s credential directory: %w", warehouseType, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s credential: %w", warehouseType, err)
	}
	if err := store.deleteAccessMarker(warehouseType, name); err != nil {
		return "", err
	}
	return path, nil
}

func (store Store) SaveAccessMarker(warehouseType, name string, marker any) error {
	path, err := store.accessMarkerPath(warehouseType, name)
	if err != nil {
		return err
	}
	if marker == nil {
		return errors.New("access marker is required")
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s access marker: %w", warehouseType, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s credential directory: %w", warehouseType, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s access marker: %w", warehouseType, err)
	}
	return nil
}

func (store Store) ReadAccessMarker(warehouseType, name string, marker any) (bool, error) {
	path, err := store.accessMarkerPath(warehouseType, name)
	if err != nil {
		return false, err
	}
	if marker == nil {
		return false, errors.New("access marker destination is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read %s access marker: %w", warehouseType, err)
	}
	if err := json.Unmarshal(data, marker); err != nil {
		return false, nil
	}
	return true, nil
}

func (store Store) accessMarkerPath(warehouseType, name string) (string, error) {
	if err := validateWarehouseType(warehouseType); err != nil {
		return "", err
	}
	if err := validateCredentialName(name); err != nil {
		return "", err
	}
	root, err := store.warehouseDir(warehouseType)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name+accessMarkerSuffix), nil
}

func (store Store) deleteAccessMarker(warehouseType, name string) error {
	path, err := store.accessMarkerPath(warehouseType, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove stale %s access marker: %w", warehouseType, err)
	}
	return nil
}

func (store Store) warehouseDir(warehouseType string) (string, error) {
	if err := validateWarehouseType(warehouseType); err != nil {
		return "", err
	}
	home, err := store.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, segmentStreamDirName, warehouseType), nil
}

func (store Store) homeDir() (string, error) {
	if store.HomeDir != "" {
		return store.HomeDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("find home directory: home directory is empty")
	}
	return home, nil
}

func validateCredentialName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("credential name is required")
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' ||
			char == '_' ||
			char == '.' {
			continue
		}
		return fmt.Errorf("invalid credential name %q; use only letters, numbers, dots, hyphens, and underscores", name)
	}
	return nil
}

func validateWarehouseType(warehouseType string) error {
	if strings.TrimSpace(warehouseType) == "" {
		return errors.New("warehouse type is required")
	}
	for _, char := range warehouseType {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' ||
			char == '_' {
			continue
		}
		return fmt.Errorf("invalid warehouse type %q; use only letters, numbers, hyphens, and underscores", warehouseType)
	}
	return nil
}
