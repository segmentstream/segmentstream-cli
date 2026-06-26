package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/cli/internal/source"
)

func TestSourceContractsJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts failed: %v", err)
	}

	var result sourceContractDetailResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.contracts" {
		t.Fatalf("command = %q, want source.contracts", response.Command)
	}
	if result.SchemaVersion != cliresult.SchemaVersion || result.Contract.Type != "events" || len(result.Columns) == 0 {
		t.Fatalf("result = %+v, want events contract with columns", result)
	}
}

func TestSourceContractsConversionsJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "conversion_events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts failed: %v", err)
	}

	var result sourceContractDetailResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.contracts" {
		t.Fatalf("command = %q, want source.contracts", response.Command)
	}
	if result.SchemaVersion != cliresult.SchemaVersion ||
		result.Contract.Type != "conversion_events" ||
		result.Model.Name != "conversion_events" ||
		len(result.Columns) != 5 {
		t.Fatalf("result = %+v, want conversion_events contract with columns", result)
	}
	if result.Columns[4].Name != "conversion_value" || result.Columns[4].Required {
		t.Fatalf("conversion value column = %+v, want optional conversion_value", result.Columns[4])
	}
}

func TestSourceScaffoldPointsToReadme(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "scaffold", "ga4", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source scaffold failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, "sources", "ga4", "README.md"))

	var result sourceScaffoldResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.scaffold" {
		t.Fatalf("command = %q, want source.scaffold", response.Command)
	}
	if len(result.Actions) != 1 ||
		result.Actions[0].Type != "read_scaffold_readme" ||
		result.Actions[0].Path == "" {
		t.Fatalf("actions = %+v, want a single README action", result.Actions)
	}
}

func TestSourceScaffoldRequiresType(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "scaffold", "ga4"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected source scaffold to require --type")
	}
}

func TestSourceVerifyRunsTemplateDbtTestsInDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	if _, err := sourcepkg.Create(root, "ga4", "events"); err != nil {
		t.Fatal(err)
	}
	withCurrentTime(t, time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC))

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "Docker Compose version v2.32.0"},
			{output: "compose built"},
			{output: "deps done"},
			{output: "tests passed"},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"source", "verify", "ga4", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source verify failed: %v", err)
	}

	runtimeDir := filepath.Join(root, ".segmentstream")
	if len(runner.calls) != 5 {
		t.Fatalf("docker calls = %v, want 5 calls", runner.calls)
	}
	assertCommand(t, runner.calls[0], "docker", []string{"info", "--format", "{{json .ServerVersion}}"}, "")
	assertCommand(t, runner.calls[1], "docker", []string{"compose", "version"}, "")
	assertCommand(t, runner.calls[2], "docker", []string{"compose", "build", "segmentstream"}, runtimeDir)
	assertCommand(t, runner.calls[3], "docker", []string{
		"compose", "run", "--rm", "--no-deps", "segmentstream",
		"dbt", "deps",
		"--project-dir", "/workspace/sources/ga4",
		"--profiles-dir", "/workspace/.segmentstream",
	}, runtimeDir)
	testArgs := runner.calls[4].Args
	for _, want := range []string{
		"compose",
		"run",
		"dbt",
		"test",
		"--project-dir",
		"/workspace/sources/ga4",
		"--profiles-dir",
		"/workspace/.segmentstream",
		"--select",
		"tag:segmentstream_source_verify",
		`"segmentstream_start_date":"2026-06-16"`,
		`"segmentstream_end_date":"2026-06-23"`,
	} {
		if !strings.Contains(strings.Join(testArgs, " "), want) {
			t.Fatalf("dbt test args = %v, want %q", testArgs, want)
		}
	}

	var result sourceVerifyResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.verify" {
		t.Fatalf("command = %q, want source.verify", response.Command)
	}
	if result.Status != "passed" ||
		result.Source != "ga4" ||
		result.StartDate != "2026-06-16" ||
		result.EndExclusiveDate != "2026-06-23" ||
		result.Fingerprint == "" {
		t.Fatalf("result = %+v, want passed verification window", result)
	}
	assertFileExists(t, filepath.Join(root, "sources", "ga4", ".segmentstream", "verification.json"))
	status, err := sourcepkg.Check(root, project.Source{Name: "ga4", Path: "./sources/ga4"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !status.Valid {
		t.Fatalf("status = %+v, want valid marker", status)
	}
}

func TestSourceRejectsRemovedSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"source", "create", "ga4"},
		{"source", "init", "ga4"},
	} {
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd := NewRootCommand(&out, &errOut)
		cmd.SetArgs(args)

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected %v to fail", args)
		}
		if !strings.Contains(err.Error(), "unknown source command") {
			t.Fatalf("error = %v, want unknown source command", err)
		}
	}
}
