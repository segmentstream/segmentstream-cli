package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureProjectReadmeCreatesReadme(t *testing.T) {
	root := t.TempDir()

	created, err := EnsureProjectReadme(root)
	if err != nil {
		t.Fatalf("EnsureProjectReadme failed: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(root, ProjectReadmeFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"SegmentStream Project",
		"Getting Started",
		"Docker Compose V2",
		"Git",
		"segmentstream warehouse auth --service-account-key",
		"~/.segmentstream/bigquery/<auth>.json",
		"Run The Pipeline",
		"produces tables in",
		"segmentstream run",
		"Create A Source",
		"segmentstream source contracts --json",
		"segmentstream source create ga4 --type events",
		"events",
		"sources/ga4/models/events.sql",
		"warehouse.auth",
		"sources/",
		".segmentstream/",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("README does not contain %q:\n%s", want, string(data))
		}
	}
}

func TestEnsureProjectReadmeDoesNotOverwriteExistingReadme(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ProjectReadmeFileName)
	if err := os.WriteFile(path, []byte("custom readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureProjectReadme(root)
	if err != nil {
		t.Fatalf("EnsureProjectReadme failed: %v", err)
	}
	if created {
		t.Fatal("created = true, want false")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom readme\n" {
		t.Fatalf("README was overwritten: %q", string(data))
	}
}

func TestEnsureAgentGuideCreatesGuide(t *testing.T) {
	root := t.TempDir()

	created, err := EnsureAgentGuide(root)
	if err != nil {
		t.Fatalf("EnsureAgentGuide failed: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true")
	}

	data, err := os.ReadFile(filepath.Join(root, AgentGuideFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "README.md") {
		t.Fatalf("agent guide does not mention README.md:\n%s", string(data))
	}
	if !strings.Contains(string(data), "segmentstream run") {
		t.Fatalf("agent guide does not mention run:\n%s", string(data))
	}
	if !strings.Contains(string(data), "segmentstream source contracts") {
		t.Fatalf("agent guide does not mention source contracts:\n%s", string(data))
	}
	if !strings.Contains(string(data), "segmentstream source create") {
		t.Fatalf("agent guide does not mention source create:\n%s", string(data))
	}
}

func TestEnsureAgentGuideDoesNotOverwriteExistingGuide(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, AgentGuideFileName)
	if err := os.WriteFile(path, []byte("custom agent guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureAgentGuide(root)
	if err != nil {
		t.Fatalf("EnsureAgentGuide failed: %v", err)
	}
	if created {
		t.Fatal("created = true, want false")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom agent guide\n" {
		t.Fatalf("agent guide was overwritten: %q", string(data))
	}
}
