package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
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

func TestSourceScaffoldPointsToImplementationGuide(t *testing.T) {
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

	assertFileExists(t, filepath.Join(root, "sources", "ga4", "IMPLEMENTATION_GUIDE.md"))

	var result sourceScaffoldResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.scaffold" {
		t.Fatalf("command = %q, want source.scaffold", response.Command)
	}
	if len(result.Actions) != 1 ||
		result.Actions[0].Type != "read_implementation_guide" ||
		result.Actions[0].Path == "" {
		t.Fatalf("actions = %+v, want a single implementation guide action", result.Actions)
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
