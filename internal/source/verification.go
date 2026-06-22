package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"gopkg.in/yaml.v3"
)

const (
	MarkerDirName  = ".segmentstream_verify"
	MarkerFileName = "last_pass.json"
	SchemaVersion  = "1"
)

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
	for _, relative := range []string{
		filepath.Join("tests", "verify_events_contract.sql"),
		filepath.Join("tests", "verify_events_non_empty.sql"),
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
