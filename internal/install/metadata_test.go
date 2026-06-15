package install

import (
	"path/filepath"
	"testing"
)

func TestMetadataRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.json")
	want := Metadata{
		Method:     MethodScript,
		InstallDir: "/tmp/bin",
		Repo:       DefaultRepo,
		Version:    "0.1.0",
		OS:         "darwin",
		Arch:       "arm64",
	}

	if err := WriteMetadata(path, want); err != nil {
		t.Fatalf("WriteMetadata failed: %v", err)
	}

	got, err := ReadMetadata(path)
	if err != nil {
		t.Fatalf("ReadMetadata failed: %v", err)
	}

	if got != want {
		t.Fatalf("metadata mismatch:\nwant: %#v\n got: %#v", want, got)
	}
}

func TestReadMissingMetadata(t *testing.T) {
	_, err := ReadMetadata(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected missing metadata error")
	}
}
