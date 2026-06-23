package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRuntimeGitignoredCreatesGitignore(t *testing.T) {
	root := t.TempDir()

	if err := EnsureRuntimeGitignored(root); err != nil {
		t.Fatalf("EnsureRuntimeGitignored failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ".segmentstream/\n" {
		t.Fatalf(".gitignore = %q, want .segmentstream entry", string(data))
	}
}

func TestEnsureRuntimeGitignoredAppendsEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("dist\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureRuntimeGitignored(root); err != nil {
		t.Fatalf("EnsureRuntimeGitignored failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "dist\n.segmentstream/\n" {
		t.Fatalf(".gitignore = %q, want appended .segmentstream entry", string(data))
	}
}

func TestEnsureRuntimeGitignoredKeepsExistingEntry(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte(".segmentstream\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureRuntimeGitignored(root); err != nil {
		t.Fatalf("EnsureRuntimeGitignored failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ".segmentstream\n" {
		t.Fatalf(".gitignore = %q, want unchanged existing entry", string(data))
	}
}
