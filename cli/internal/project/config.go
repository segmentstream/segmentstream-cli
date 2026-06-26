package project

import (
	"errors"
	"fmt"
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
	Sources   []Source  `yaml:"sources,omitempty"`
	Identity  *Identity `yaml:"identity,omitempty"`
}

type Requires struct {
	SegmentStream string `yaml:"segmentstream,omitempty"`
}

type Warehouse struct {
	Type     string `yaml:"type,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
	Project  string `yaml:"project,omitempty"`
	Dataset  string `yaml:"dataset,omitempty"`
	Location string `yaml:"location,omitempty"`
}

type Source struct {
	Name        string `yaml:"name"`
	Path        string `yaml:"path"`
	PackageName string `yaml:"package_name,omitempty"`
}

type Identity struct {
	Keys []IdentityKey `yaml:"keys,omitempty"`
}

type IdentityKey struct {
	Name                    string `yaml:"name"`
	Tier                    string `yaml:"tier"`
	WindowDays              int    `yaml:"window_days"`
	MaxDistinctAnonymousIDs int    `yaml:"max_distinct_anonymous_ids"`
	Scope                   string `yaml:"scope"`
}

type rawConfig struct {
	Version   *int      `yaml:"version"`
	Requires  Requires  `yaml:"requires"`
	Warehouse Warehouse `yaml:"warehouse"`
	Sources   []Source  `yaml:"sources"`
	Identity  *Identity `yaml:"identity"`
}

func DefaultConfigYAML() string {
	return `version: 1

warehouse:
  type: bigquery
  auth: default-bigquery

# sources:
#   - name: ga4
#     path: ./sources/ga4
#   - name: crm_conversions
#     path: ./sources/crm_conversions
#   - name: sdk_identity
#     path: ./sources/sdk_identity
#
# identity:
#   keys:
#     - name: user_id
#       tier: deterministic
#       window_days: 180
#       max_distinct_anonymous_ids: 1000
#       scope: project
`
}

func LoadConfig(projectRoot string) (Config, error) {
	return (Store{Root: projectRoot}).Load()
}

func ParsePartialConfig(data []byte) (Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}

	config := Config{
		Requires: normalizeRequires(raw.Requires),
		Warehouse: Warehouse{
			Type:     strings.TrimSpace(raw.Warehouse.Type),
			Auth:     strings.TrimSpace(raw.Warehouse.Auth),
			Project:  strings.TrimSpace(raw.Warehouse.Project),
			Dataset:  strings.TrimSpace(raw.Warehouse.Dataset),
			Location: strings.TrimSpace(raw.Warehouse.Location),
		},
		Sources:  normalizeSources(raw.Sources),
		Identity: normalizeIdentity(raw.Identity),
	}
	if raw.Version != nil {
		config.Version = *raw.Version
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
		Sources:  normalizeSources(raw.Sources),
		Identity: normalizeIdentity(raw.Identity),
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

	if config.Warehouse.Project == "your-gcp-project" {
		return errors.New("warehouse.project still contains placeholder value your-gcp-project")
	}

	if err := validateIdentity(config.Identity); err != nil {
		return err
	}

	return nil
}

func normalizeRequires(requires Requires) Requires {
	return Requires{
		SegmentStream: strings.TrimSpace(requires.SegmentStream),
	}
}

func normalizeSources(sources []Source) []Source {
	if len(sources) == 0 {
		return nil
	}

	normalized := make([]Source, 0, len(sources))
	for _, source := range sources {
		normalized = append(normalized, Source{
			Name:        strings.TrimSpace(source.Name),
			Path:        strings.TrimSpace(source.Path),
			PackageName: strings.TrimSpace(source.PackageName),
		})
	}
	return normalized
}

func normalizeIdentity(identity *Identity) *Identity {
	if identity == nil || len(identity.Keys) == 0 {
		return nil
	}

	keys := make([]IdentityKey, 0, len(identity.Keys))
	for _, key := range identity.Keys {
		keys = append(keys, IdentityKey{
			Name:                    strings.TrimSpace(key.Name),
			Tier:                    strings.TrimSpace(key.Tier),
			WindowDays:              key.WindowDays,
			MaxDistinctAnonymousIDs: key.MaxDistinctAnonymousIDs,
			Scope:                   strings.TrimSpace(key.Scope),
		})
	}
	return &Identity{Keys: keys}
}

func validateIdentity(identity *Identity) error {
	if identity == nil {
		return nil
	}

	seen := map[string]struct{}{}
	for index, key := range identity.Keys {
		field := fmt.Sprintf("identity.keys[%d]", index)
		if key.Name == "" {
			return fmt.Errorf("missing required field %s.name", field)
		}
		if strings.ContainsAny(key.Name, "\n\r") {
			return fmt.Errorf("%s.name must not contain newlines", field)
		}
		if _, exists := seen[key.Name]; exists {
			return fmt.Errorf("duplicate identity key %q", key.Name)
		}
		seen[key.Name] = struct{}{}

		switch key.Tier {
		case "deterministic", "probabilistic":
		case "":
			return fmt.Errorf("missing required field %s.tier", field)
		default:
			return fmt.Errorf("%s.tier must be deterministic or probabilistic", field)
		}

		if key.WindowDays <= 0 {
			return fmt.Errorf("%s.window_days must be a positive integer", field)
		}
		if key.MaxDistinctAnonymousIDs <= 0 {
			return fmt.Errorf("%s.max_distinct_anonymous_ids must be a positive integer", field)
		}

		switch key.Scope {
		case "project", "source":
		case "":
			return fmt.Errorf("missing required field %s.scope", field)
		default:
			return fmt.Errorf("%s.scope must be project or source", field)
		}
	}

	return nil
}
