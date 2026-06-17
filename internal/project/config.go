package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ConfigFileName         = "segmentstream.yml"
	SupportedConfigVersion = 1
	DefaultLocation        = "US"
)

type Config struct {
	Version   int       `yaml:"version"`
	Requires  Requires  `yaml:"requires,omitempty"`
	Warehouse Warehouse `yaml:"warehouse"`
}

type Requires struct {
	SegmentStream string `yaml:"segmentstream,omitempty"`
}

type Warehouse struct {
	Type     string `yaml:"type"`
	Auth     string `yaml:"auth"`
	Project  string `yaml:"project"`
	Dataset  string `yaml:"dataset"`
	Location string `yaml:"location,omitempty"`
}

type rawConfig struct {
	Version   *int      `yaml:"version"`
	Requires  Requires  `yaml:"requires"`
	Warehouse Warehouse `yaml:"warehouse"`
}

func DefaultConfigYAML() string {
	return `version: 1

warehouse:
  type: bigquery
  auth: default-bigquery
  project: your-gcp-project
  dataset: segmentstream
  location: US

# sources:
#   - name: ga4
#     path: ./sources/ga4
`
}

func LoadConfig(projectRoot string) (Config, error) {
	path := filepath.Join(projectRoot, ConfigFileName)
	data, err := os.ReadFile(path)
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

func ParseConfig(data []byte) (Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}
	if raw.Version == nil {
		return Config{}, errors.New("missing required field version")
	}

	config := Config{
		Version:  *raw.Version,
		Requires: normalizeRequires(raw.Requires),
		Warehouse: Warehouse{
			Type:     strings.TrimSpace(raw.Warehouse.Type),
			Auth:     strings.TrimSpace(raw.Warehouse.Auth),
			Project:  strings.TrimSpace(raw.Warehouse.Project),
			Dataset:  strings.TrimSpace(raw.Warehouse.Dataset),
			Location: strings.TrimSpace(raw.Warehouse.Location),
		},
	}
	if config.Warehouse.Location == "" {
		config.Warehouse.Location = DefaultLocation
	}

	if err := ValidateConfig(config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func ValidateConfig(config Config) error {
	if config.Version != SupportedConfigVersion {
		return fmt.Errorf("unsupported version %d; this CLI supports version %d", config.Version, SupportedConfigVersion)
	}
	if config.Warehouse.Type == "" {
		return errors.New("missing required field warehouse.type")
	}
	if config.Warehouse.Type != "bigquery" {
		return fmt.Errorf("unsupported warehouse.type %q; only bigquery is supported in config version 1", config.Warehouse.Type)
	}

	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "warehouse.auth", value: config.Warehouse.Auth},
		{name: "warehouse.project", value: config.Warehouse.Project},
		{name: "warehouse.dataset", value: config.Warehouse.Dataset},
	} {
		if required.value == "" {
			return fmt.Errorf("missing required field %s", required.name)
		}
	}

	return nil
}

func normalizeRequires(requires Requires) Requires {
	return Requires{
		SegmentStream: strings.TrimSpace(requires.SegmentStream),
	}
}
