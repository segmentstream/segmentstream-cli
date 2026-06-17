package projectsource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/templates"
)

const SourcesDirName = "sources"

var sourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Source struct {
	Name        string
	PackageName string
	Path        string
}

type templateData struct {
	Name        string
	PackageName string
}

func Init(projectRoot, name string) (Source, error) {
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
	if err := copySourceTemplate(target, data); err != nil {
		return Source{}, err
	}

	return Source{
		Name:        name,
		PackageName: data.PackageName,
		Path:        target,
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

func copySourceTemplate(targetDir string, data templateData) error {
	const root = "source"

	return fs.WalkDir(templates.Source, root, func(templatePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if templatePath == root {
			return nil
		}

		relative := strings.TrimPrefix(templatePath, root+"/")
		relative = renderSourceTemplateText(relative, data)
		relative = filepath.FromSlash(relative)
		target := filepath.Join(targetDir, relative)

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
		return nil
	})
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
