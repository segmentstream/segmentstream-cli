package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/templates"
	"gopkg.in/yaml.v3"
)

const SourcesDirName = "sources"

const contractsRoot = "source/contracts"

var sourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Source struct {
	Name         string
	PackageName  string
	Path         string
	Contract     ContractIdentity
	Model        ContractModel
	Columns      []ContractColumn
	CreatedFiles []string
}

type ContractIdentity struct {
	Type          string `json:"type" yaml:"type"`
	SchemaVersion int    `json:"schema_version" yaml:"schema_version"`
}

type Contract struct {
	Type          string              `json:"type" yaml:"type"`
	SchemaVersion int                 `json:"schema_version" yaml:"schema_version"`
	Description   string              `json:"description" yaml:"description"`
	Default       bool                `json:"default" yaml:"default"`
	Status        string              `json:"status" yaml:"status"`
	Model         ContractModel       `json:"model" yaml:"model"`
	Columns       []ContractColumn    `json:"columns" yaml:"columns"`
	Migrations    []ContractMigration `json:"migrations,omitempty" yaml:"migrations"`
	templateDir   string
}

type ContractMigration struct {
	From  int    `json:"from" yaml:"from"`
	To    int    `json:"to" yaml:"to"`
	Guide string `json:"guide" yaml:"guide"`
}

type ContractModel struct {
	Name      string `json:"name" yaml:"name"`
	Partition string `json:"partition" yaml:"partition"`
}

type ContractColumn struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Required    bool   `json:"required" yaml:"required"`
	Description string `json:"description" yaml:"description"`
}

type templateData struct {
	Name        string
	PackageName string
}

type ContractValidationContext struct {
	SourceName string
	SourcePath string
}

type ContractMigrationRequiredError struct {
	ContractType      string `json:"contract_type"`
	FromSchemaVersion int    `json:"from_schema_version"`
	ToSchemaVersion   int    `json:"to_schema_version"`
	SourceName        string `json:"source_name,omitempty"`
	SourcePath        string `json:"source_path,omitempty"`
	GuidePath         string `json:"guide_path,omitempty"`
	MigrationGuide    string `json:"migration_guide"`
	NextCommand       string `json:"next_command,omitempty"`
}

func (err *ContractMigrationRequiredError) Error() string {
	var message strings.Builder
	if err.SourceName != "" {
		fmt.Fprintf(&message, "source %q contract %s schema_version %d is unsupported; expected schema_version %d.", err.SourceName, err.ContractType, err.FromSchemaVersion, err.ToSchemaVersion)
	} else {
		fmt.Fprintf(&message, "source contract %s schema_version %d is unsupported; expected schema_version %d.", err.ContractType, err.FromSchemaVersion, err.ToSchemaVersion)
	}
	if err.GuidePath != "" {
		fmt.Fprintf(&message, "\n\nMigration guide (%s):", err.GuidePath)
	} else {
		message.WriteString("\n\nMigration guide:")
	}
	guide := strings.TrimSpace(err.MigrationGuide)
	if guide != "" {
		fmt.Fprintf(&message, "\n\n%s", guide)
	}
	if err.NextCommand != "" {
		fmt.Fprintf(&message, "\n\nNext action: %s", err.NextCommand)
	}
	return message.String()
}

func (err *ContractMigrationRequiredError) Diagnostics() []cliresult.Diagnostic {
	field := "contract.schema_version"
	if err.SourceName != "" {
		field = fmt.Sprintf("sources.%s.contract.schema_version", err.SourceName)
	}
	return []cliresult.Diagnostic{
		{
			ID:         "source_contract_migration_required",
			Field:      field,
			Message:    err.Error(),
			Suggestion: "Apply the migration guide, then verify the source again.",
		},
	}
}

func (err *ContractMigrationRequiredError) Actions() []cliresult.Action {
	if err.NextCommand == "" {
		return nil
	}
	return []cliresult.Action{
		{
			Type:    "run_command",
			Label:   "Verify migrated source",
			Command: err.NextCommand,
		},
	}
}

func (err *ContractMigrationRequiredError) ErrorData() any {
	return err
}

func AsContractMigrationRequired(err error) (*ContractMigrationRequiredError, bool) {
	var migrationErr *ContractMigrationRequiredError
	if errors.As(err, &migrationErr) {
		return migrationErr, true
	}
	return nil, false
}

func Create(projectRoot, name, contractType string) (Source, error) {
	contract, err := ContractByType(contractType)
	if err != nil {
		return Source{}, err
	}
	return createWithContract(projectRoot, name, contract)
}

func Contracts() ([]Contract, error) {
	entries, err := fs.ReadDir(templates.Source, contractsRoot)
	if err != nil {
		return nil, fmt.Errorf("read source contracts: %w", err)
	}

	var contracts []Contract
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		templateDir := contractsRoot + "/" + entry.Name()
		contract, err := readContract(templateDir)
		if err != nil {
			return nil, err
		}
		if contract.Status != "supported" {
			continue
		}
		contracts = append(contracts, contract)
	}

	sort.Slice(contracts, func(i, j int) bool {
		if contracts[i].Type == contracts[j].Type {
			return contracts[i].SchemaVersion < contracts[j].SchemaVersion
		}
		return contracts[i].Type < contracts[j].Type
	})
	return contracts, nil
}

func ContractByType(contractType string) (Contract, error) {
	contractType = strings.TrimSpace(contractType)
	if contractType == "" {
		return Contract{}, errors.New("source contract type is required")
	}

	contracts, err := Contracts()
	if err != nil {
		return Contract{}, err
	}
	for _, contract := range contracts {
		if contract.Type == contractType {
			return contract, nil
		}
	}
	return Contract{}, unknownContractError(contractType, contracts)
}

func DefaultContract() (Contract, error) {
	contracts, err := Contracts()
	if err != nil {
		return Contract{}, err
	}

	var defaults []Contract
	for _, contract := range contracts {
		if contract.Default {
			defaults = append(defaults, contract)
		}
	}
	if len(defaults) == 0 {
		return Contract{}, errors.New("no default source contract is embedded")
	}
	if len(defaults) > 1 {
		var names []string
		for _, contract := range defaults {
			names = append(names, contract.Type)
		}
		return Contract{}, fmt.Errorf("multiple default source contracts are embedded: %s", strings.Join(names, ", "))
	}
	return defaults[0], nil
}

func (contract Contract) Identity() ContractIdentity {
	return ContractIdentity{
		Type:          contract.Type,
		SchemaVersion: contract.SchemaVersion,
	}
}

func ValidateSupportedContractIdentity(contract ContractIdentity) error {
	return ValidateSupportedContractIdentityForSource(contract, ContractValidationContext{})
}

func ValidateSupportedContractIdentityForSource(contract ContractIdentity, context ContractValidationContext) error {
	embedded, err := ContractByType(contract.Type)
	if err != nil {
		return err
	}
	if contract.SchemaVersion != embedded.SchemaVersion {
		if contract.SchemaVersion < embedded.SchemaVersion {
			guide, ok, err := contractMigrationGuide(embedded, contract.SchemaVersion, embedded.SchemaVersion, context)
			if err != nil {
				return err
			}
			if ok {
				return &ContractMigrationRequiredError{
					ContractType:      contract.Type,
					FromSchemaVersion: contract.SchemaVersion,
					ToSchemaVersion:   embedded.SchemaVersion,
					SourceName:        strings.TrimSpace(context.SourceName),
					SourcePath:        filepath.ToSlash(strings.TrimSpace(context.SourcePath)),
					GuidePath:         guide.GuidePath,
					MigrationGuide:    guide.Body,
					NextCommand:       sourceVerifyCommand(context.SourceName),
				}
			}
		}
		if contract.SchemaVersion > embedded.SchemaVersion {
			return fmt.Errorf(
				"source contract %s schema_version %d is newer than this CLI supports; this CLI supports schema_version %d. Upgrade segmentstream CLI, then rerun verification",
				contract.Type,
				contract.SchemaVersion,
				embedded.SchemaVersion,
			)
		}
		return fmt.Errorf(
			"source contract %s schema_version %d is unsupported; expected schema_version %d. No migration guide is embedded for %s %d to %d; inspect segmentstream source contracts --type %s and update contract.yml, models/schema.yml, models/%s.sql, and verification tests to the latest contract",
			contract.Type,
			contract.SchemaVersion,
			embedded.SchemaVersion,
			contract.Type,
			contract.SchemaVersion,
			embedded.SchemaVersion,
			contract.Type,
			embedded.Model.Name,
		)
	}
	return nil
}

type loadedContractMigrationGuide struct {
	GuidePath string
	Body      string
}

func contractMigrationGuide(contract Contract, fromVersion, toVersion int, context ContractValidationContext) (loadedContractMigrationGuide, bool, error) {
	migrations, ok := resolveContractMigrationPath(contract, fromVersion, toVersion)
	if !ok {
		return loadedContractMigrationGuide{}, false, nil
	}

	var guidePaths []string
	var sections []string
	for _, migration := range migrations {
		guidePath := contract.templateDir + "/" + path.Clean(migration.Guide)
		data, err := fs.ReadFile(templates.Source, guidePath)
		if err != nil {
			return loadedContractMigrationGuide{}, false, fmt.Errorf("read source contract migration guide %s: %w", guidePath, err)
		}
		guidePaths = append(guidePaths, guidePath)
		sections = append(sections, renderContractMigrationGuide(string(data), contract, migration, context))
	}

	return loadedContractMigrationGuide{
		GuidePath: strings.Join(guidePaths, ", "),
		Body:      strings.Join(sections, "\n\n---\n\n"),
	}, true, nil
}

func resolveContractMigrationPath(contract Contract, fromVersion, toVersion int) ([]ContractMigration, bool) {
	if fromVersion >= toVersion {
		return nil, false
	}

	var migrations []ContractMigration
	current := fromVersion
	for current < toVersion {
		var candidates []ContractMigration
		for _, migration := range contract.Migrations {
			if migration.From == current && migration.To > current && migration.To <= toVersion {
				candidates = append(candidates, migration)
			}
		}
		if len(candidates) == 0 {
			return nil, false
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].To < candidates[j].To
		})
		selected := candidates[0]
		migrations = append(migrations, selected)
		current = selected.To
	}
	return migrations, true
}

func renderContractMigrationGuide(text string, contract Contract, migration ContractMigration, context ContractValidationContext) string {
	sourceName := strings.TrimSpace(context.SourceName)
	if sourceName == "" {
		sourceName = "<source-name>"
	}
	sourcePath := filepath.ToSlash(strings.TrimSpace(context.SourcePath))
	if sourcePath == "" {
		sourcePath = filepath.ToSlash(filepath.Join(SourcesDirName, sourceName))
	}
	replacements := map[string]string{
		"__SOURCE_NAME__":         sourceName,
		"__SOURCE_PATH__":         sourcePath,
		"__CONTRACT_TYPE__":       contract.Type,
		"__FROM_SCHEMA_VERSION__": fmt.Sprintf("%d", migration.From),
		"__TO_SCHEMA_VERSION__":   fmt.Sprintf("%d", migration.To),
		"__MODEL_NAME__":          contract.Model.Name,
	}
	for token, value := range replacements {
		text = strings.ReplaceAll(text, token, value)
	}
	return strings.TrimSpace(text)
}

func sourceVerifyCommand(sourceName string) string {
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" {
		sourceName = "<source-name>"
	}
	return fmt.Sprintf("segmentstream source verify %s", sourceName)
}

func createWithContract(projectRoot, name string, contract Contract) (Source, error) {
	name = strings.TrimSpace(name)
	if err := ValidateName(name); err != nil {
		return Source{}, err
	}
	if err := ensureProjectExists(projectRoot); err != nil {
		return Source{}, err
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return Source{}, fmt.Errorf("resolve project root: %w", err)
	}

	target := filepath.Join(root, SourcesDirName, name)
	if err := validateSourceDir(root, name, target); err != nil {
		return Source{}, err
	}
	if _, err := os.Stat(target); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Source{}, fmt.Errorf("check source directory %s: %w", filepath.Join(SourcesDirName, name), err)
		}
	} else {
		return Source{}, fmt.Errorf("source %q already exists at %s", name, filepath.Join(SourcesDirName, name))
	}

	data := templateData{
		Name:        name,
		PackageName: "segmentstream_source_" + name,
	}
	createdFiles, err := copySourceTemplate(target, contract, data)
	if err != nil {
		return Source{}, err
	}

	return Source{
		Name:         name,
		PackageName:  data.PackageName,
		Path:         target,
		Contract:     contract.Identity(),
		Model:        contract.Model,
		Columns:      append([]ContractColumn(nil), contract.Columns...),
		CreatedFiles: createdFiles,
	}, nil
}

func ValidateName(name string) error {
	if name == "" {
		return errors.New("source name is required")
	}
	if !sourceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid source name %q; use lowercase letters, numbers, and underscores, starting with a letter", name)
	}
	return nil
}

func ensureProjectExists(projectRoot string) error {
	path := filepath.Join(projectRoot, project.ConfigFileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s was not found in the current directory; run segmentstream init first", project.ConfigFileName)
		}
		return fmt.Errorf("check %s: %w", project.ConfigFileName, err)
	}
	return nil
}

func validateSourceDir(projectRoot, name, sourceDir string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	target, err := filepath.Abs(sourceDir)
	if err != nil {
		return fmt.Errorf("resolve source directory: %w", err)
	}
	expected := filepath.Join(root, SourcesDirName, name)
	if filepath.Clean(target) != filepath.Clean(expected) {
		return fmt.Errorf("refusing to write source directory %s; expected %s", target, expected)
	}
	return nil
}

func copySourceTemplate(targetDir string, contract Contract, data templateData) ([]string, error) {
	var createdFiles []string
	err := fs.WalkDir(templates.Source, contract.templateDir, func(templatePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if templatePath == contract.templateDir {
			return nil
		}

		relative := strings.TrimPrefix(templatePath, contract.templateDir+"/")
		if isContractMetadataPath(relative) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		relative = renderSourceTemplateText(relative, data)
		target := filepath.Join(targetDir, filepath.FromSlash(relative))

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		contents, err := fs.ReadFile(templates.Source, templatePath)
		if err != nil {
			return fmt.Errorf("read source template %s: %w", templatePath, err)
		}
		rendered := []byte(renderSourceTemplateText(string(contents), data))

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", target, err)
		}
		if err := os.WriteFile(target, rendered, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		createdFiles = append(createdFiles, filepath.ToSlash(filepath.Join(SourcesDirName, data.Name, filepath.FromSlash(relative))))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdFiles, nil
}

func isContractMetadataPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	return relative == "migrations" || strings.HasPrefix(relative, "migrations/")
}

func renderSourceTemplateText(text string, data templateData) string {
	replacements := map[string]string{
		"__SOURCE_NAME__":  data.Name,
		"__PACKAGE_NAME__": data.PackageName,
	}
	for token, value := range replacements {
		text = strings.ReplaceAll(text, token, value)
	}
	return text
}

func readContract(templateDir string) (Contract, error) {
	path := templateDir + "/contract.yml"
	data, err := fs.ReadFile(templates.Source, path)
	if err != nil {
		return Contract{}, fmt.Errorf("read source contract %s: %w", path, err)
	}

	var contract Contract
	if err := yaml.Unmarshal(data, &contract); err != nil {
		return Contract{}, fmt.Errorf("parse source contract %s: %w", path, err)
	}
	contract.templateDir = templateDir
	if err := validateContract(contract, templateDir); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func validateContract(contract Contract, templateDir string) error {
	if strings.TrimSpace(contract.Type) == "" {
		return fmt.Errorf("source contract %s/contract.yml is missing type", templateDir)
	}
	if contract.SchemaVersion <= 0 {
		return fmt.Errorf("source contract %s/contract.yml is missing schema_version", templateDir)
	}
	if contract.Model.Name == "" {
		return fmt.Errorf("source contract %s/contract.yml is missing model.name", templateDir)
	}
	if contract.Model.Partition == "" {
		return fmt.Errorf("source contract %s/contract.yml is missing model.partition", templateDir)
	}
	if len(contract.Columns) == 0 {
		return fmt.Errorf("source contract %s/contract.yml must declare columns", templateDir)
	}
	if filepath.Base(templateDir) != contract.Type {
		return fmt.Errorf("source contract %s/contract.yml type %q does not match directory name", templateDir, contract.Type)
	}
	for _, migration := range contract.Migrations {
		if migration.From <= 0 {
			return fmt.Errorf("source contract %s/contract.yml migration is missing from", templateDir)
		}
		if migration.To <= migration.From {
			return fmt.Errorf("source contract %s/contract.yml migration %d must move to a newer schema_version", templateDir, migration.From)
		}
		guide := strings.TrimSpace(migration.Guide)
		if guide == "" {
			return fmt.Errorf("source contract %s/contract.yml migration %d to %d is missing guide", templateDir, migration.From, migration.To)
		}
		cleanGuide := path.Clean(guide)
		if strings.HasPrefix(cleanGuide, "../") || cleanGuide == ".." || strings.HasPrefix(cleanGuide, "/") {
			return fmt.Errorf("source contract %s/contract.yml migration guide %q must stay inside the contract template", templateDir, migration.Guide)
		}
		guidePath := templateDir + "/" + cleanGuide
		if _, err := fs.Stat(templates.Source, guidePath); err != nil {
			return fmt.Errorf("source contract %s/contract.yml migration guide %s is not embedded: %w", templateDir, guidePath, err)
		}
	}
	return nil
}

func unknownContractError(contractType string, contracts []Contract) error {
	var types []string
	for _, contract := range contracts {
		types = append(types, contract.Type)
	}
	if len(types) == 0 {
		return fmt.Errorf("unknown source contract type %q; no source contracts are supported by this CLI", contractType)
	}
	return fmt.Errorf("unknown source contract type %q; supported types: %s", contractType, strings.Join(types, ", "))
}
