package projectruntime

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/templates"
)

const RuntimeDirName = ".segmentstream"

func Prepare(projectRoot string, config project.Config, provider warehouse.Provider) error {
	if provider == nil {
		return errors.New("prepare runtime: warehouse provider is required")
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	hostHome, err := hostSegmentStreamHome()
	if err != nil {
		return err
	}
	runtimeDir := filepath.Join(root, RuntimeDirName)
	if err := validateRuntimeDir(root, runtimeDir); err != nil {
		return err
	}

	if err := os.RemoveAll(runtimeDir); err != nil {
		return fmt.Errorf("remove %s: %w", RuntimeDirName, err)
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", RuntimeDirName, err)
	}

	if err := copyProjectTemplate(runtimeDir); err != nil {
		return err
	}
	if err := writeRuntimeEnv(runtimeDir, config, hostHome, provider); err != nil {
		return err
	}
	if err := writeDBTProfile(runtimeDir, config, provider); err != nil {
		return err
	}
	if err := ensureRuntimeDirs(runtimeDir); err != nil {
		return err
	}
	return nil
}

func hostSegmentStreamHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("find user home directory: home directory is empty")
	}

	path, err := filepath.Abs(filepath.Join(home, ".segmentstream"))
	if err != nil {
		return "", fmt.Errorf("resolve user SegmentStream directory: %w", err)
	}
	return filepath.ToSlash(path), nil
}

func validateRuntimeDir(projectRoot, runtimeDir string) error {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	target, err := filepath.Abs(runtimeDir)
	if err != nil {
		return fmt.Errorf("resolve runtime directory: %w", err)
	}
	expected := filepath.Join(root, RuntimeDirName)
	if filepath.Clean(target) != filepath.Clean(expected) {
		return fmt.Errorf("refusing to remove runtime directory %s; expected %s", target, expected)
	}
	return nil
}

func copyProjectTemplate(runtimeDir string) error {
	const root = "project"

	return fs.WalkDir(templates.Project, root, func(templatePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if templatePath == root {
			return nil
		}

		relative := strings.TrimPrefix(templatePath, root+"/")
		relative = filepath.FromSlash(relative)
		target := filepath.Join(runtimeDir, relative)

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		contents, err := fs.ReadFile(templates.Project, templatePath)
		if err != nil {
			return fmt.Errorf("read template %s: %w", templatePath, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", target, err)
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

func writeRuntimeEnv(runtimeDir string, config project.Config, hostHome string, provider warehouse.Provider) error {
	env := []warehouse.EnvVar{
		{Name: "SEGMENTSTREAM_HOST_HOME", Value: hostHome},
	}
	env = append(env, provider.RuntimeEnvironment(config.Warehouse)...)

	var output strings.Builder
	for _, item := range env {
		if item.Name == "" {
			return errors.New("write runtime environment: provider returned an empty environment variable name")
		}
		if strings.ContainsAny(item.Name, "\r\n=") {
			return fmt.Errorf("write runtime environment: invalid environment variable name %q", item.Name)
		}
		if strings.ContainsAny(item.Value, "\r\n") {
			return fmt.Errorf("write runtime environment: %s contains a newline", item.Name)
		}
		fmt.Fprintf(&output, "%s=%s\n", item.Name, strconv.Quote(item.Value))
	}

	path := filepath.Join(runtimeDir, ".env")
	if err := os.WriteFile(path, []byte(output.String()), 0o600); err != nil {
		return fmt.Errorf("write runtime environment: %w", err)
	}
	return nil
}

func writeDBTProfile(runtimeDir string, config project.Config, provider warehouse.Provider) error {
	profile := provider.DBTProfileYAML(config.Warehouse)
	if strings.TrimSpace(profile) == "" {
		return errors.New("write dbt profile: provider returned an empty profile")
	}
	if !strings.HasSuffix(profile, "\n") {
		profile += "\n"
	}
	path := filepath.Join(runtimeDir, "profiles.yml")
	if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
		return fmt.Errorf("write dbt profile: %w", err)
	}
	return nil
}

func ensureRuntimeDirs(runtimeDir string) error {
	for _, dir := range []string{
		filepath.Join("dbt", "macros"),
		filepath.Join("dbt", "models", "exports"),
		filepath.Join("dbt", "models", "staging"),
		filepath.Join("dbt", "seeds"),
		filepath.Join("dbt", "snapshots"),
		filepath.Join("dbt", "tests"),
		filepath.Join("logs"),
		filepath.Join("target"),
	} {
		if err := os.MkdirAll(filepath.Join(runtimeDir, dir), 0o755); err != nil {
			return fmt.Errorf("create runtime directory %s: %w", dir, err)
		}
	}
	return nil
}
