package project

import (
	"path/filepath"
	"testing"
)

func TestStoreWriteDefaultAndLoad(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}

	exists, err := store.Exists()
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Fatal("Exists = true, want false before default config is written")
	}

	if err := store.WriteDefault(); err != nil {
		t.Fatalf("WriteDefault failed: %v", err)
	}

	exists, err = store.Exists()
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Fatal("Exists = false, want true after default config is written")
	}

	config, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Warehouse.Type != "bigquery" {
		t.Fatalf("Warehouse.Type = %q, want bigquery", config.Warehouse.Type)
	}
}

func TestStoreSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}

	want := Config{
		Version: SupportedConfigVersion,
		Warehouse: Warehouse{
			Type:     "bigquery",
			Auth:     "production-bigquery",
			Project:  "example-project",
			Dataset:  "segmentstream",
			Location: "EU",
		},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.Warehouse != want.Warehouse {
		t.Fatalf("Warehouse = %+v, want %+v", got.Warehouse, want.Warehouse)
	}
}

func TestStoreConfigPath(t *testing.T) {
	root := t.TempDir()
	got := (Store{Root: root}).ConfigPath()
	want := filepath.Join(root, ConfigFileName)
	if got != want {
		t.Fatalf("ConfigPath = %q, want %q", got, want)
	}
}
