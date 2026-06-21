package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/segmentstream/segmentstream-cli/internal/install"
	"github.com/segmentstream/segmentstream-cli/internal/version"
)

type Options struct {
	CheckOnly bool
}

type Result struct {
	Status          string `json:"status"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
	CheckOnly       bool   `json:"check_only"`
	Asset           string `json:"asset,omitempty"`
}

type Updater struct {
	CurrentVersion string
	GOOS           string
	GOARCH         string
	MetadataPath   string
	ReleaseClient  ReleaseClient
	Out            io.Writer
	ErrOut         io.Writer
}

func NewUpdater(info version.Info, out, errOut io.Writer) Updater {
	metadataPath, err := install.DefaultMetadataPath()
	if err != nil {
		metadataPath = ""
	}

	return Updater{
		CurrentVersion: info.Version,
		GOOS:           info.OS,
		GOARCH:         info.Arch,
		MetadataPath:   metadataPath,
		ReleaseClient:  ReleaseClient{},
		Out:            out,
		ErrOut:         errOut,
	}
}

func (updater Updater) Run(ctx context.Context, options Options) error {
	_, err := updater.run(ctx, options, updater.Out)
	return err
}

func (updater Updater) RunWithResult(ctx context.Context, options Options) (Result, error) {
	return updater.run(ctx, options, io.Discard)
}

func (updater Updater) run(ctx context.Context, options Options, out io.Writer) (Result, error) {
	if out == nil {
		out = io.Discard
	}

	metadataPath := updater.MetadataPath
	if metadataPath == "" {
		var err error
		metadataPath, err = install.DefaultMetadataPath()
		if err != nil {
			return Result{}, err
		}
	}

	metadata, err := install.ReadMetadata(metadataPath)
	if err != nil {
		return Result{}, err
	}
	if metadata.Method != install.MethodScript {
		return Result{}, fmt.Errorf("install method %q cannot be self-updated; reinstall with install.sh", metadata.Method)
	}
	if metadata.InstallDir == "" {
		return Result{}, errors.New("install metadata does not include install_dir; reinstall with install.sh")
	}

	repo := metadata.Repo
	if repo == "" {
		repo = install.DefaultRepo
	}

	release, err := updater.ReleaseClient.LatestRelease(ctx, repo)
	if err != nil {
		return Result{}, err
	}
	latestVersion := normalizeVersion(release.TagName)
	currentVersion := metadata.Version
	if currentVersion == "" {
		currentVersion = updater.CurrentVersion
	}

	comparison, err := compareVersions(currentVersion, latestVersion)
	if err != nil {
		return Result{}, fmt.Errorf("compare versions: %w", err)
	}
	result := Result{
		Status:          "up_to_date",
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: comparison < 0,
		CheckOnly:       options.CheckOnly,
	}

	fmt.Fprintf(out, "Current version: %s\n", currentVersion)
	fmt.Fprintf(out, "Latest version:  %s\n", latestVersion)

	if comparison >= 0 {
		fmt.Fprintln(out, "segmentstream is already up to date.")
		return result, nil
	}

	if options.CheckOnly {
		result.Status = "update_available"
		fmt.Fprintln(out, "An update is available.")
		return result, nil
	}

	goos := updater.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := updater.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	assetName := assetName(goos, goarch)
	archiveAsset, err := findAsset(release, assetName)
	if err != nil {
		return Result{}, err
	}
	checksumAsset, err := findAsset(release, "checksums.txt")
	if err != nil {
		return Result{}, err
	}
	result.Status = "updated"
	result.Asset = assetName

	fmt.Fprintf(out, "Downloading %s\n", assetName)
	checksums, err := updater.ReleaseClient.Download(ctx, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return Result{}, err
	}
	wantChecksum, err := checksumForAsset(checksums, assetName)
	if err != nil {
		return Result{}, err
	}
	archiveBytes, err := updater.ReleaseClient.Download(ctx, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp("", "segmentstream-update-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, assetName)
	if err := os.WriteFile(archivePath, archiveBytes, 0o644); err != nil {
		return Result{}, fmt.Errorf("write downloaded archive: %w", err)
	}

	fmt.Fprintln(out, "Verifying checksum")
	if err := verifyFileChecksum(archivePath, wantChecksum); err != nil {
		return Result{}, err
	}

	extracted, err := extractBinaryFromTarGz(archivePath, tempDir)
	if err != nil {
		return Result{}, err
	}

	targetPath := filepath.Join(metadata.InstallDir, "segmentstream")
	if err := ensureReplaceAllowed(targetPath); err != nil {
		return Result{}, err
	}

	fmt.Fprintln(out, "Installing update")
	if err := replaceBinary(targetPath, extracted.Path, extracted.Mode); err != nil {
		return Result{}, err
	}

	metadata.Version = latestVersion
	metadata.OS = goos
	metadata.Arch = goarch
	if err := install.WriteMetadata(metadataPath, metadata); err != nil {
		return Result{}, err
	}

	result.Updated = true
	fmt.Fprintf(out, "Updated segmentstream to %s\n", latestVersion)
	return result, nil
}

func ensureReplaceAllowed(targetPath string) error {
	file, err := os.OpenFile(targetPath, os.O_WRONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("cannot update %s: permission denied; rerun install.sh with --install-dir set to a user-writable directory", targetPath)
		}
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot update %s: installed binary was not found; reinstall with install.sh", targetPath)
		}
		return fmt.Errorf("cannot update %s: %w", targetPath, err)
	}
	return file.Close()
}

func replaceBinary(targetPath, sourcePath string, mode os.FileMode) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read extracted binary: %w", err)
	}
	if mode == 0 {
		mode = 0o755
	}
	mode |= 0o111

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), ".segmentstream-update-*")
	if err != nil {
		return fmt.Errorf("create replacement binary: %w", err)
	}
	tempPath := tempFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("write replacement binary: %w", err)
	}
	if err := tempFile.Chmod(mode); err != nil {
		tempFile.Close()
		return fmt.Errorf("set replacement permissions: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close replacement binary: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("replace installed binary: %w", err)
	}
	cleanup = false
	return nil
}
