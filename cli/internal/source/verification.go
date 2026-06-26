package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"gopkg.in/yaml.v3"
)

const (
	MarkerDirName  = ".segmentstream"
	MarkerFileName = "verification.json"
	SchemaVersion  = "1"

	DefaultVerifyDays = 7
)

var verifyProgressInterval = 15 * time.Second

type Marker struct {
	SchemaVersion    string           `json:"schema_version"`
	Source           string           `json:"source"`
	Contract         ContractIdentity `json:"contract"`
	StartDate        string           `json:"start_date"`
	EndExclusiveDate string           `json:"end_exclusive_date"`
	VerifiedAt       string           `json:"verified_at"`
	Fingerprint      string           `json:"fingerprint"`
}

type Status struct {
	Valid       bool
	Reason      string
	MarkerPath  string
	Fingerprint string
	Contract    ContractIdentity
}

type VerifyRequest struct {
	ProjectRoot        string
	SourceName         string
	StartDate          string
	Runner             CommandRunner
	Progress           Progress
	WarehousePreflight func(project.Config) error
	RuntimePreflight   func() error
	PrepareRuntime     func(projectRoot string, config project.Config) error
	Now                func() time.Time
}

type VerifyResult struct {
	Source           string
	SourcePath       string
	Status           string
	StartDate        string
	EndExclusiveDate string
	EndInclusiveDate string
	MarkerPath       string
	Fingerprint      string
}

type CommandInvocation struct {
	Name string
	Args []string
	Dir  string
}

type CommandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, invocation CommandInvocation) (string, error)
}

type Progress interface {
	Start(message string)
	Detail(message string)
	OK(message string)
	StillWorking(message string, elapsed time.Duration)
}

type verifyDateRange struct {
	StartDate        string
	EndExclusiveDate string
}

func Verify(ctx context.Context, request VerifyRequest) (VerifyResult, error) {
	sourceName := strings.TrimSpace(request.SourceName)
	if sourceName == "" {
		return VerifyResult{}, errors.New("source name is required")
	}
	if strings.TrimSpace(request.ProjectRoot) == "" {
		return VerifyResult{}, errors.New("project root is required")
	}
	if request.Runner == nil {
		return VerifyResult{}, errors.New("source verification runner is required")
	}
	if request.PrepareRuntime == nil {
		return VerifyResult{}, errors.New("source verification runtime preparer is required")
	}

	now := request.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	progress := request.Progress
	if progress == nil {
		progress = noopProgress{}
	}

	config, err := project.LoadConfig(request.ProjectRoot)
	if err != nil {
		return VerifyResult{}, err
	}
	source, ok := findConfiguredSource(config, sourceName)
	if !ok {
		return VerifyResult{}, fmt.Errorf("source %q is not declared in %s", sourceName, project.ConfigFileName)
	}
	verifyRange, err := resolveVerifyDateRange(request.StartDate, now())
	if err != nil {
		return VerifyResult{}, err
	}
	sourcePath, err := ResolveSourcePath(request.ProjectRoot, source)
	if err != nil {
		return VerifyResult{}, err
	}
	contract, contractOK, err := readSourceContractSnapshot(sourcePath)
	if err != nil {
		return VerifyResult{}, err
	}
	if !contractOK {
		return VerifyResult{}, errors.New("source contract snapshot is missing")
	}
	if err := ValidateSupportedContractIdentityForSource(contract, ContractValidationContext{
		SourceName: source.Name,
		SourcePath: sourcePath,
	}); err != nil {
		return VerifyResult{}, err
	}
	if err := RequireTemplateTests(sourcePath); err != nil {
		return VerifyResult{}, err
	}
	containerSourcePath, err := ContainerSourcePath(request.ProjectRoot, source)
	if err != nil {
		return VerifyResult{}, err
	}
	if request.WarehousePreflight != nil {
		if err := request.WarehousePreflight(config); err != nil {
			return VerifyResult{}, err
		}
	}

	progress.Start("Checking local environment")
	if request.RuntimePreflight != nil {
		if err := request.RuntimePreflight(); err != nil {
			return VerifyResult{}, err
		}
	}
	if err := preflightDocker(ctx, request.Runner); err != nil {
		return VerifyResult{}, err
	}
	progress.OK("")

	progress.Start("Preparing project files")
	if err := request.PrepareRuntime(request.ProjectRoot, config); err != nil {
		return VerifyResult{}, err
	}
	progress.OK("")

	runtimeDir := filepath.Join(request.ProjectRoot, MarkerDirName)
	progress.Start("Building verification container")
	output, err := runWithProgress(ctx, progress, request.Runner, CommandInvocation{
		Name: "docker",
		Args: []string{"compose", "build", "segmentstream"},
		Dir:  runtimeDir,
	}, "Still building verification container")
	if err != nil {
		return VerifyResult{}, commandError("Source verification failed while building the SegmentStream runtime.", output, err)
	}
	progress.OK("")

	progress.Start("Running source dbt tests")
	progress.Detail(fmt.Sprintf("Verifying %s from %s through %s", source.Name, verifyRange.StartDate, verifyEndInclusiveDate(verifyRange)))
	vars, err := json.Marshal(map[string]string{
		"segmentstream_start_date": verifyRange.StartDate,
		"segmentstream_end_date":   verifyRange.EndExclusiveDate,
	})
	if err != nil {
		return VerifyResult{}, fmt.Errorf("marshal dbt vars: %w", err)
	}
	if output, err = runWithProgress(ctx, progress, request.Runner, sourceVerifyDockerInvocation(runtimeDir, []string{
		"dbt",
		"deps",
		"--project-dir", containerSourcePath,
		"--profiles-dir", "/workspace/.segmentstream",
	}), "Still installing source dbt dependencies"); err != nil {
		return VerifyResult{}, commandError("Source verification failed while installing dbt dependencies.", output, err)
	}
	if output, err = runWithProgress(ctx, progress, request.Runner, sourceVerifyDockerInvocation(runtimeDir, []string{
		"dbt",
		"test",
		"--project-dir", containerSourcePath,
		"--profiles-dir", "/workspace/.segmentstream",
		"--select", "tag:segmentstream_source_verify",
		"--vars", string(vars),
	}), "Still running source dbt tests"); err != nil {
		return VerifyResult{}, commandError("Source verification failed.", output, err)
	}

	marker, markerPath, err := SavePassing(request.ProjectRoot, source, verifyRange.StartDate, verifyRange.EndExclusiveDate, now())
	if err != nil {
		return VerifyResult{}, err
	}
	progress.OK("")

	return VerifyResult{
		Source:           source.Name,
		SourcePath:       filepath.ToSlash(source.Path),
		Status:           "passed",
		StartDate:        verifyRange.StartDate,
		EndExclusiveDate: verifyRange.EndExclusiveDate,
		EndInclusiveDate: verifyEndInclusiveDate(verifyRange),
		MarkerPath:       filepath.ToSlash(markerPath),
		Fingerprint:      marker.Fingerprint,
	}, nil
}

func Check(projectRoot string, source project.Source) (Status, error) {
	sourcePath, err := ResolveSourcePath(projectRoot, source)
	if err != nil {
		return Status{Valid: false, Reason: err.Error()}, nil
	}
	markerPath := MarkerPath(sourcePath)

	contract, contractOK, err := readSourceContractSnapshot(sourcePath)
	if err != nil {
		return Status{}, err
	}
	if !contractOK {
		return Status{Valid: false, Reason: "source contract snapshot is missing", MarkerPath: markerPath}, nil
	}
	if err := ValidateSupportedContractIdentityForSource(contract, ContractValidationContext{
		SourceName: source.Name,
		SourcePath: sourcePath,
	}); err != nil {
		return Status{Valid: false, Reason: err.Error(), MarkerPath: markerPath, Contract: contract}, nil
	}

	fingerprint, err := Fingerprint(sourcePath)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		MarkerPath:  markerPath,
		Fingerprint: fingerprint,
		Contract:    contract,
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.Reason = "source has not passed verification"
			return status, nil
		}
		return Status{}, fmt.Errorf("read source verification marker: %w", err)
	}

	var marker Marker
	if err := json.Unmarshal(data, &marker); err != nil {
		status.Reason = "source verification marker is invalid"
		return status, nil
	}
	if marker.SchemaVersion != SchemaVersion {
		status.Reason = "source verification marker uses an unsupported schema version"
		return status, nil
	}
	if marker.Source != source.Name {
		status.Reason = "source verification marker belongs to a different source"
		return status, nil
	}
	if marker.Contract != contract {
		status.Reason = "source contract changed since verification"
		return status, nil
	}
	if marker.Fingerprint != fingerprint {
		status.Reason = "source files changed since verification"
		return status, nil
	}

	status.Valid = true
	return status, nil
}

func SavePassing(projectRoot string, source project.Source, startDate, endExclusiveDate string, verifiedAt time.Time) (Marker, string, error) {
	sourcePath, err := ResolveSourcePath(projectRoot, source)
	if err != nil {
		return Marker{}, "", err
	}
	contract, ok, err := readSourceContractSnapshot(sourcePath)
	if err != nil {
		return Marker{}, "", err
	}
	if !ok {
		return Marker{}, "", errors.New("source contract snapshot is missing")
	}
	if err := ValidateSupportedContractIdentityForSource(contract, ContractValidationContext{
		SourceName: source.Name,
		SourcePath: sourcePath,
	}); err != nil {
		return Marker{}, "", err
	}
	fingerprint, err := Fingerprint(sourcePath)
	if err != nil {
		return Marker{}, "", err
	}
	marker := Marker{
		SchemaVersion:    SchemaVersion,
		Source:           source.Name,
		Contract:         contract,
		StartDate:        startDate,
		EndExclusiveDate: endExclusiveDate,
		VerifiedAt:       verifiedAt.UTC().Format(time.RFC3339),
		Fingerprint:      fingerprint,
	}

	markerPath := MarkerPath(sourcePath)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return Marker{}, "", fmt.Errorf("create source verification marker directory: %w", err)
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return Marker{}, "", fmt.Errorf("marshal source verification marker: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(markerPath, data, 0o644); err != nil {
		return Marker{}, "", fmt.Errorf("write source verification marker: %w", err)
	}
	return marker, markerPath, nil
}

func ResolveSourcePath(projectRoot string, source project.Source) (string, error) {
	if strings.TrimSpace(source.Path) == "" {
		return "", fmt.Errorf("source %q does not declare a path", source.Name)
	}

	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	path := source.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("source %q path is outside the project root", source.Name)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("source %q path does not exist", source.Name)
		}
		return "", fmt.Errorf("check source path: %w", err)
	}
	return path, nil
}

func ContainerSourcePath(projectRoot string, source project.Source) (string, error) {
	sourcePath, err := ResolveSourcePath(projectRoot, source)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	relative, err := filepath.Rel(root, sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve source container path: %w", err)
	}
	return "/workspace/" + filepath.ToSlash(relative), nil
}

func MarkerPath(sourcePath string) string {
	return filepath.Join(sourcePath, MarkerDirName, MarkerFileName)
}

func RequireTemplateTests(sourcePath string) error {
	modelName, err := verificationModelName(sourcePath)
	if err != nil {
		return err
	}
	for _, relative := range []string{
		filepath.Join("tests", fmt.Sprintf("verify_%s_contract.sql", modelName)),
		filepath.Join("tests", fmt.Sprintf("verify_%s_non_empty.sql", modelName)),
	} {
		path := filepath.Join(sourcePath, relative)
		if info, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("source verification test %s is missing; run segmentstream source scaffold again or restore the template test", filepath.ToSlash(relative))
			}
			return fmt.Errorf("check source verification test %s: %w", filepath.ToSlash(relative), err)
		} else if info.IsDir() {
			return fmt.Errorf("source verification test %s is a directory", filepath.ToSlash(relative))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read source verification test %s: %w", filepath.ToSlash(relative), err)
		}
		if !strings.Contains(string(data), "segmentstream_source_verify") {
			return fmt.Errorf("source verification test %s must keep the segmentstream_source_verify tag", filepath.ToSlash(relative))
		}
	}
	return nil
}

func verificationModelName(sourcePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(sourcePath, "contract.yml"))
	if err != nil {
		return "", fmt.Errorf("read source contract snapshot: %w", err)
	}

	var contract Contract
	if err := yaml.Unmarshal(data, &contract); err != nil {
		return "", fmt.Errorf("parse source contract snapshot: %w", err)
	}
	modelName := strings.TrimSpace(contract.Model.Name)
	if modelName != "" {
		if !sourceNamePattern.MatchString(modelName) {
			return "", fmt.Errorf("source contract model name %q is invalid", modelName)
		}
		return modelName, nil
	}

	if strings.TrimSpace(contract.Type) == "" {
		return "", errors.New("source contract snapshot is incomplete")
	}
	embedded, err := ContractByType(contract.Type)
	if err != nil {
		return "", err
	}
	if !sourceNamePattern.MatchString(embedded.Model.Name) {
		return "", fmt.Errorf("embedded source contract model name %q is invalid", embedded.Model.Name)
	}
	return embedded.Model.Name, nil
}

func sourceVerifyDockerInvocation(runtimeDir string, args []string) CommandInvocation {
	dockerArgs := []string{"compose", "run", "--rm", "--no-deps", "segmentstream"}
	dockerArgs = append(dockerArgs, args...)
	return CommandInvocation{
		Name: "docker",
		Args: dockerArgs,
		Dir:  runtimeDir,
	}
}

func resolveVerifyDateRange(startDate string, now time.Time) (verifyDateRange, error) {
	today := utcDate(now)
	start := today.AddDate(0, 0, -(DefaultVerifyDays - 1))
	if strings.TrimSpace(startDate) != "" {
		parsed, err := parseDate(startDate, "--start-date")
		if err != nil {
			return verifyDateRange{}, err
		}
		start = parsed
	}
	if start.After(today) {
		return verifyDateRange{}, fmt.Errorf("--start-date %s is after current UTC date %s", formatDate(start), formatDate(today))
	}
	return verifyDateRange{
		StartDate:        formatDate(start),
		EndExclusiveDate: formatDate(today.AddDate(0, 0, 1)),
	}, nil
}

func verifyEndInclusiveDate(dateRange verifyDateRange) string {
	end, err := parseDate(dateRange.EndExclusiveDate, "end_date")
	if err != nil {
		return dateRange.EndExclusiveDate
	}
	return formatDate(end.AddDate(0, 0, -1))
}

func findConfiguredSource(config project.Config, sourceName string) (project.Source, bool) {
	for _, source := range config.Sources {
		if source.Name == sourceName {
			return source, true
		}
	}
	return project.Source{}, false
}

func preflightDocker(ctx context.Context, runner CommandRunner) error {
	if _, err := runner.LookPath("docker"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("Docker is required to run source verification. Install Docker Desktop or Docker Engine and make sure docker is on PATH.")
		}
		return fmt.Errorf("check Docker CLI: %w", err)
	}

	if output, err := runner.Run(ctx, CommandInvocation{Name: "docker", Args: []string{"info", "--format", "{{json .ServerVersion}}"}}); err != nil {
		return commandError("Docker is installed, but Docker Engine is not running or this user cannot access it.", output, err)
	}

	if output, err := runner.Run(ctx, CommandInvocation{Name: "docker", Args: []string{"compose", "version"}}); err != nil {
		return commandError("Docker Compose V2 is required. Install or update Docker so 'docker compose' is available.", output, err)
	}

	return nil
}

func runWithProgress(ctx context.Context, progress Progress, runner CommandRunner, invocation CommandInvocation, progressMessage string) (string, error) {
	type commandResult struct {
		output string
		err    error
	}

	done := make(chan commandResult, 1)
	startedAt := time.Now()
	go func() {
		output, err := runner.Run(ctx, invocation)
		done <- commandResult{output: output, err: err}
	}()

	ticker := time.NewTicker(verifyProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case result := <-done:
			return result.output, result.err
		case <-ticker.C:
			progress.StillWorking(progressMessage, time.Since(startedAt))
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func commandError(message, output string, err error) error {
	output = strings.TrimSpace(output)
	if output != "" {
		return fmt.Errorf("%s\n\nDetails:\n%s", message, output)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return errors.New(message)
}

func parseDate(value, name string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q; use YYYY-MM-DD", name, value)
	}
	return utcDate(parsed), nil
}

func utcDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func formatDate(value time.Time) string {
	return value.UTC().Format("2006-01-02")
}

type noopProgress struct{}

func (noopProgress) Start(message string)                               {}
func (noopProgress) Detail(message string)                              {}
func (noopProgress) OK(message string)                                  {}
func (noopProgress) StillWorking(message string, elapsed time.Duration) {}

func Fingerprint(sourcePath string) (string, error) {
	var files []string
	for _, relative := range []string{
		"contract.yml",
		"dbt_project.yml",
		"models",
		"tests",
	} {
		path := filepath.Join(sourcePath, relative)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("check source fingerprint path %s: %w", relative, err)
		}
		if !info.IsDir() {
			files = append(files, relative)
			continue
		}
		if err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relativePath, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relativePath))
			return nil
		}); err != nil {
			return "", fmt.Errorf("walk source fingerprint path %s: %w", relative, err)
		}
	}
	sort.Strings(files)

	hash := sha256.New()
	for _, relative := range files {
		data, err := os.ReadFile(filepath.Join(sourcePath, filepath.FromSlash(relative)))
		if err != nil {
			return "", fmt.Errorf("read source fingerprint file %s: %w", relative, err)
		}
		hash.Write([]byte(relative))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSourceContractSnapshot(sourcePath string) (ContractIdentity, bool, error) {
	data, err := os.ReadFile(filepath.Join(sourcePath, "contract.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ContractIdentity{}, false, nil
		}
		return ContractIdentity{}, false, fmt.Errorf("read source contract snapshot: %w", err)
	}
	var contract ContractIdentity
	if err := yaml.Unmarshal(data, &contract); err != nil {
		return ContractIdentity{}, false, fmt.Errorf("parse source contract snapshot: %w", err)
	}
	if strings.TrimSpace(contract.Type) == "" || contract.SchemaVersion <= 0 {
		return ContractIdentity{}, false, errors.New("source contract snapshot is incomplete")
	}
	return contract, true, nil
}
