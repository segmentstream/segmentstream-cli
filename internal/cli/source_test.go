package cli

import (
	"bytes"
	"encoding/json"
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

	var result struct {
		SchemaVersion string `json:"schema_version"`
		Contract      struct {
			Type string `json:"type"`
		} `json:"contract"`
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("source contracts output is not JSON: %v\n%s", err, out.String())
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

	var result struct {
		Actions []struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("source scaffold output is not JSON: %v\n%s", err, out.String())
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
