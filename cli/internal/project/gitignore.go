package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const RuntimeGitignoreEntry = ".segmentstream/"

func EnsureRuntimeGitignored(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.WriteFile(path, []byte(RuntimeGitignoreEntry+"\n"), 0o644)
		}
		return fmt.Errorf("read .gitignore: %w", err)
	}

	if hasRuntimeGitignoreEntry(string(data)) {
		return nil
	}

	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += RuntimeGitignoreEntry + "\n"

	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	return nil
}

func hasRuntimeGitignoreEntry(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == RuntimeGitignoreEntry || trimmed == ".segmentstream" {
			return true
		}
	}
	return false
}
