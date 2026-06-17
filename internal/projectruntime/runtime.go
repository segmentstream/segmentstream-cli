package projectruntime

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/segmentstream/segmentstream-cli/internal/project"
)

const RuntimeDirName = ".segmentstream"

//go:embed templates/default/**
var templateFS embed.FS

type templateData struct {
	Config                project.Config
	Warehouse             project.Warehouse
	HostSegmentStreamHome string
}

func Prepare(projectRoot string, config project.Config) error {
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

	data := templateData{
		Config:                config,
		Warehouse:             config.Warehouse,
		HostSegmentStreamHome: hostHome,
	}
	if err := renderTemplates(runtimeDir, data); err != nil {
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
	return yamlQuote(filepath.ToSlash(path)), nil
}

func yamlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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

func renderTemplates(runtimeDir string, data templateData) error {
	const root = "templates/default"

	return fs.WalkDir(templateFS, root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve template path %s: %w", path, err)
		}
		relative = filepath.FromSlash(relative)
		target := filepath.Join(runtimeDir, strings.TrimSuffix(relative, ".tmpl"))

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		contents, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read template %s: %w", path, err)
		}
		rendered, err := renderTemplate(path, contents, data)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", target, err)
		}
		if err := os.WriteFile(target, rendered, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

func renderTemplate(name string, contents []byte, data templateData) ([]byte, error) {
	parsed, err := template.New(name).Parse(string(contents))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}

	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	return output.Bytes(), nil
}

func ensureRuntimeDirs(runtimeDir string) error {
	for _, dir := range []string{
		filepath.Join("dbt", "analyses"),
		filepath.Join("dbt", "macros"),
		filepath.Join("dbt", "models"),
		filepath.Join("dbt", "seeds"),
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
