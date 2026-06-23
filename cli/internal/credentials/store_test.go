package credentials

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type testAccessMarker struct {
	Project string `json:"project"`
	Dataset string `json:"dataset"`
}

func TestCredentialPath(t *testing.T) {
	home := t.TempDir()
	path, err := (Store{HomeDir: home}).CredentialPath("testwarehouse", "default")
	if err != nil {
		t.Fatalf("CredentialPath failed: %v", err)
	}
	want := filepath.Join(home, ".segmentstream", "testwarehouse", "default.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestSaveCredentialData(t *testing.T) {
	root := t.TempDir()
	data := []byte(`{"type":"credential","secret":"value"}`)

	path, err := (Store{HomeDir: filepath.Join(root, "home")}).SaveCredentialData("testwarehouse", "default", data)
	if err != nil {
		t.Fatalf("SaveCredentialData failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("stored credential = %s, want source credential", string(got))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSaveCredentialDataClearsAccessMarker(t *testing.T) {
	root := t.TempDir()
	store := Store{HomeDir: filepath.Join(root, "home")}
	if err := store.SaveAccessMarker("testwarehouse", "default", testAccessMarker{Project: "example-project", Dataset: "segmentstream"}); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	var before testAccessMarker
	found, err := store.ReadAccessMarker("testwarehouse", "default", &before)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected marker before replacing credential")
	}

	if _, err := store.SaveCredentialData("testwarehouse", "default", []byte(`{"type":"credential"}`)); err != nil {
		t.Fatalf("SaveCredentialData failed: %v", err)
	}

	var after testAccessMarker
	found, err = store.ReadAccessMarker("testwarehouse", "default", &after)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected marker to be cleared after replacing credential")
	}
}

func TestReadAccessMarker(t *testing.T) {
	home := t.TempDir()
	store := Store{HomeDir: home}
	if err := store.SaveAccessMarker("testwarehouse", "default", testAccessMarker{Project: "example-project", Dataset: "segmentstream"}); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	var marker testAccessMarker
	found, err := store.ReadAccessMarker("testwarehouse", "default", &marker)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected marker to exist")
	}
	if marker.Project != "example-project" || marker.Dataset != "segmentstream" {
		t.Fatalf("marker = %+v, want saved marker", marker)
	}
}

func TestCredentialPathRejectsInvalidWarehouseType(t *testing.T) {
	_, err := (Store{HomeDir: t.TempDir()}).CredentialPath("../warehouse", "default")
	if err == nil {
		t.Fatal("expected invalid warehouse type error")
	}
}
