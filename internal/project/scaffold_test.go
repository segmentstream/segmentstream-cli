package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldCreatesProjectFiles(t *testing.T) {
	root := t.TempDir()

	result, err := Scaffold(root)
	if err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	if !result.ConfigCreated || result.ConfigExisted {
		t.Fatalf("result = %+v, want created config", result)
	}
	if !result.ReadmeCreated {
		t.Fatalf("result = %+v, want readme created", result)
	}
	if !result.AgentGuideCreated {
		t.Fatalf("result = %+v, want agent guide created", result)
	}

	assertProjectFileExists(t, filepath.Join(root, ConfigFileName))
	assertProjectFileExists(t, filepath.Join(root, ProjectReadmeFileName))
	assertProjectFileExists(t, filepath.Join(root, AgentGuideFileName))
	assertProjectFileExists(t, filepath.Join(root, ".gitignore"))
}

func TestScaffoldKeepsExistingProjectFiles(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ConfigFileName)
	readmePath := filepath.Join(root, ProjectReadmeFileName)
	agentGuidePath := filepath.Join(root, AgentGuideFileName)

	if err := os.WriteFile(configPath, []byte("custom config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readmePath, []byte("custom readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentGuidePath, []byte("custom agent guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Scaffold(root)
	if err != nil {
		t.Fatalf("Scaffold failed: %v", err)
	}

	if result.ConfigCreated || !result.ConfigExisted {
		t.Fatalf("result = %+v, want existing config", result)
	}
	if result.ReadmeCreated {
		t.Fatalf("result = %+v, did not want readme created", result)
	}
	if result.AgentGuideCreated {
		t.Fatalf("result = %+v, did not want agent guide created", result)
	}
	assertProjectFileContents(t, configPath, "custom config\n")
	assertProjectFileContents(t, readmePath, "custom readme\n")
	assertProjectFileContents(t, agentGuidePath, "custom agent guide\n")
}

func assertProjectFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func assertProjectFileContents(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}
