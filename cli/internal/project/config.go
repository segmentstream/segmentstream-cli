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
}

type rawConfig struct {
	Version   *int      `yaml:"version"`
	Requires  Requires  `yaml:"requires"`
	Warehouse Warehouse `yaml:"warehouse"`
	Sources   []Source  `yaml:"sources"`
	Identity  *Identity `yaml:"identity"`
}

func DefaultConfigYAML() string {
	return "version: 1\n" +
		"\n" +
		"warehouse:\n" +
		"  type: bigquery\n" +
		"  auth: default-bigquery\n" +
		"\n" +
		"# sources:\n" +
		"#   - name: ga4\n" +
		"#     path: ./sources/ga4\n" +
		"#   - name: crm_conversion_events\n" +
		"#     path: ./sources/crm_conversion_events\n" +
		"#   - name: sdk_identity\n" +
		"#     path: ./sources/sdk_identity\n" +
		"#\n" +
		"# identity:\n" +
		"#   keys:\n" +
		"#     - name: user_id\n" +
		"#       tier: deterministic\n" +
		"#       window_days: 180\n" +
		"#       max_distinct_anonymous_ids: 1000\n"
}

func LoadConfig(projectRoot string) (Config, error) {
	return (Store{Root: projectRoot}).Load()
}

func ParsePartialConfig(data []byte) (Config, error) {
	raw, err := decodeRawConfig(data)
	if err != nil {
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
	raw, err := decodeRawConfig(data)
	if err != nil {
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

func decodeRawConfig(data []byte) (rawConfig, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return rawConfig{}, err
	}
	if err := rejectUnsupportedIdentityScope(root); err != nil {
		return rawConfig{}, err
	}

	var raw rawConfig
	if err := root.Decode(&raw); err != nil {
		return rawConfig{}, err
	}
	return raw, nil
}

func rejectUnsupportedIdentityScope(root yaml.Node) error {
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = *root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}

	identity := yamlMappingValue(&root, "identity")
	if identity == nil || identity.Kind != yaml.MappingNode {
		return nil
	}
	keys := yamlMappingValue(identity, "keys")
	if keys == nil || keys.Kind != yaml.SequenceNode {
		return nil
	}

	for index, key := range keys.Content {
		if key.Kind != yaml.MappingNode {
			continue
		}
		if yamlMappingValue(key, "scope") != nil {
			return fmt.Errorf("identity.keys[%d].scope is no longer supported; identity keys are matched globally", index)
		}
	}
	return nil
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
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
	}

	return nil
}
