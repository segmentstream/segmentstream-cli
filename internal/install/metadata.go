package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	MethodScript = "script"
	DefaultRepo  = "segmentstream/segmentstream-cli"
)

type Metadata struct {
	Method     string `json:"method"`
	InstallDir string `json:"install_dir"`
	Repo       string `json:"repo"`
	Version    string `json:"version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

func DefaultMetadataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".segmentstream", "install.json"), nil
}

func ReadMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Metadata{}, fmt.Errorf("install metadata was not found at %s; reinstall with install.sh before using segmentstream update", path)
		}
		return Metadata{}, fmt.Errorf("read install metadata: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, fmt.Errorf("parse install metadata: %w", err)
	}
	return metadata, nil
}

func WriteMetadata(path string, metadata Metadata) error {
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install metadata: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write install metadata: %w", err)
	}
	return nil
}
