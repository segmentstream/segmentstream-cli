package initflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

func TestEvaluateAsksForWarehouseWithoutMutating(t *testing.T) {
	root := t.TempDir()

	result, err := (Service{
		ProjectRoot: root,
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitNeedsUserDecision {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitNeedsUserDecision)
	}
	if result.Envelope.NextAction.Type != "ask_user" {
		t.Fatalf("next action = %+v, want ask_user", result.Envelope.NextAction)
	}
	if _, err := os.Stat(filepath.Join(root, project.ConfigFileName)); !os.IsNotExist(err) {
		t.Fatalf("initflow mutated config, stat err = %v", err)
	}
}

func TestEvaluateSelectsWarehouseThenNeedsAuth(t *testing.T) {
	root := t.TempDir()

	result, err := (Service{
		ProjectRoot: root,
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	}).Evaluate(context.Background(), Options{SelectWarehouse: "bigquery"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitNeedsAuth)
	}
	config, _, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Type != "bigquery" || config.Warehouse.Auth != "default-bigquery" {
		t.Fatalf("warehouse = %+v, want selected bigquery", config.Warehouse)
	}
	if config.Warehouse.Project != "" {
		t.Fatalf("warehouse project = %q, want no placeholder", config.Warehouse.Project)
	}
}

func TestEvaluateNeedsConfigurationAfterAuth(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeCredential(t, home, "default-bigquery")

	result, err := (Service{
		ProjectRoot: root,
		Credentials: credentials.Store{HomeDir: home},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitMisconfigured)
	}
	if result.Envelope.NextAction.Command != "segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>" {
		t.Fatalf("next action = %+v", result.Envelope.NextAction)
	}
	if len(result.Envelope.NextAction.Hints) != 1 {
		t.Fatalf("hints = %+v, want one browse hint", result.Envelope.NextAction.Hints)
	}
	hint := result.Envelope.NextAction.Hints[0]
	if hint.ID != "browse_warehouse_before_configure" {
		t.Fatalf("hint id = %q, want browse_warehouse_before_configure", hint.ID)
	}
	if len(hint.Commands) != 2 ||
		hint.Commands[0] != "segmentstream warehouse browse --json" ||
		hint.Commands[1] != "segmentstream warehouse browse --path <project> --json" {
		t.Fatalf("hint commands = %+v, want browse commands", hint.Commands)
	}
}

func TestEvaluateReadyAfterAccessMarker(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := (project.Store{Root: root}).Save(project.Config{
		Version: project.SupportedConfigVersion,
		Warehouse: project.Warehouse{
			Type:     "bigquery",
			Auth:     "default-bigquery",
			Project:  "example-project",
			Dataset:  "segmentstream",
			Location: "EU",
		},
	}); err != nil {
		t.Fatal(err)
	}
	writeCredential(t, home, "default-bigquery")
	if err := (credentials.Store{HomeDir: home}).SaveAccessMarker("default-bigquery", "example-project", "segmentstream", "EU"); err != nil {
		t.Fatal(err)
	}

	result, err := (Service{
		ProjectRoot: root,
		Credentials: credentials.Store{HomeDir: home},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitReady || !result.Envelope.Ready {
		t.Fatalf("result = %+v, want ready", result)
	}
	if result.Envelope.NextAction.Type != "done" {
		t.Fatalf("next action = %+v, want done", result.Envelope.NextAction)
	}
}

func writeCredential(t *testing.T, home, name string) {
	t.Helper()
	path, err := (credentials.Store{HomeDir: home}).BigQueryCredentialPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}
