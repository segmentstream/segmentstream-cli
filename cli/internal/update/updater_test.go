package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/cli/internal/install"
)

func TestUpdateCheckAvailable(t *testing.T) {
	releaseClient := newTestReleaseClient(t, "v0.2.0", []byte("new binary"))

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "install.json")
	installDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "segmentstream"), []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestMetadata(t, metadataPath, installDir, "0.1.0")

	var out bytes.Buffer
	updater := Updater{
		CurrentVersion: "0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		MetadataPath:   metadataPath,
		ReleaseClient:  releaseClient,
		Out:            &out,
	}

	if err := updater.Run(context.Background(), Options{CheckOnly: true}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), "An update is available.") {
		t.Fatalf("output did not report update: %q", out.String())
	}
}

func TestUpdateNoUpdateAvailable(t *testing.T) {
	releaseClient := newTestReleaseClient(t, "v0.1.0", []byte("same binary"))

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "install.json")
	installDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "segmentstream"), []byte("same binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestMetadata(t, metadataPath, installDir, "0.1.0")

	var out bytes.Buffer
	updater := Updater{
		CurrentVersion: "0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		MetadataPath:   metadataPath,
		ReleaseClient:  releaseClient,
		Out:            &out,
	}

	if err := updater.Run(context.Background(), Options{}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Fatalf("output did not report up to date: %q", out.String())
	}
}

func TestUpdateInstallsNewBinary(t *testing.T) {
	releaseClient := newTestReleaseClient(t, "v0.2.0", []byte("new binary"))

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "install.json")
	installDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(installDir, "segmentstream")
	if err := os.WriteFile(targetPath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestMetadata(t, metadataPath, installDir, "0.1.0")

	var out bytes.Buffer
	updater := Updater{
		CurrentVersion: "0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		MetadataPath:   metadataPath,
		ReleaseClient:  releaseClient,
		Out:            &out,
	}

	if err := updater.Run(context.Background(), Options{}); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new binary" {
		t.Fatalf("installed binary = %q, want new binary", string(data))
	}

	metadata, err := install.ReadMetadata(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Version != "0.2.0" {
		t.Fatalf("metadata version = %q, want 0.2.0", metadata.Version)
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	releaseClient := newTestReleaseClientWithChecksum(t, "v0.2.0", []byte("new binary"), "bad")

	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "install.json")
	installDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "segmentstream"), []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestMetadata(t, metadataPath, installDir, "0.1.0")

	var out bytes.Buffer
	updater := Updater{
		CurrentVersion: "0.1.0",
		GOOS:           "darwin",
		GOARCH:         "arm64",
		MetadataPath:   metadataPath,
		ReleaseClient:  releaseClient,
		Out:            &out,
	}

	err := updater.Run(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
}

func TestUpdateRefusesUnknownInstallMethod(t *testing.T) {
	tempDir := t.TempDir()
	metadataPath := filepath.Join(tempDir, "install.json")
	if err := install.WriteMetadata(metadataPath, install.Metadata{Method: "brew", InstallDir: tempDir, Repo: install.DefaultRepo, Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}

	updater := Updater{MetadataPath: metadataPath}
	err := updater.Run(context.Background(), Options{CheckOnly: true})
	if err == nil {
		t.Fatal("expected unknown install method error")
	}
	if !strings.Contains(err.Error(), "cannot be self-updated") {
		t.Fatalf("error = %v, want self-update refusal", err)
	}
}

func writeTestMetadata(t *testing.T, path, installDir, version string) {
	t.Helper()
	if err := install.WriteMetadata(path, install.Metadata{
		Method:     install.MethodScript,
		InstallDir: installDir,
		Repo:       install.DefaultRepo,
		Version:    version,
		OS:         "darwin",
		Arch:       "arm64",
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func newTestReleaseClient(t *testing.T, tag string, binary []byte) ReleaseClient {
	archive := makeArchive(t, binary)
	sum := sha256.Sum256(archive)
	return newTestReleaseClientWithChecksum(t, tag, binary, hex.EncodeToString(sum[:]))
}

func newTestReleaseClientWithChecksum(t *testing.T, tag string, binary []byte, checksum string) ReleaseClient {
	t.Helper()
	archive := makeArchive(t, binary)
	asset := "segmentstream_darwin_arm64.tar.gz"
	baseURL := "https://segmentstream.test"

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/segmentstream/segmentstream/releases/latest":
			return testResponse(http.StatusOK, fmt.Sprintf(`{
			"tag_name": %q,
			"prerelease": false,
			"assets": [
				{"name": %q, "browser_download_url": "%s/download/%s"},
				{"name": "checksums.txt", "browser_download_url": "%s/download/checksums.txt"}
			]
		}`, tag, asset, baseURL, asset, baseURL)), nil
		case "/download/" + asset:
			return testBinaryResponse(http.StatusOK, archive), nil
		case "/download/checksums.txt":
			return testResponse(http.StatusOK, fmt.Sprintf("%s  %s\n", checksum, asset)), nil
		default:
			return testResponse(http.StatusNotFound, "not found"), nil
		}
	})}

	return ReleaseClient{BaseURL: baseURL, HTTPClient: client}
}

func makeArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	header := &tar.Header{
		Name: "segmentstream",
		Mode: 0o755,
		Size: int64(len(binary)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func testResponse(status int, body string) *http.Response {
	return testBinaryResponse(status, []byte(body))
}

func testBinaryResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}
