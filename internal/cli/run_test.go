package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunFailsWhenConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	if !strings.Contains(err.Error(), "segmentstream.yml was not found") {
		t.Fatalf("error = %v, want missing config message", err)
	}
	if len(runner.lookups) != 0 || len(runner.calls) != 0 {
		t.Fatalf("docker was called before config was loaded: lookups=%v calls=%v", runner.lookups, runner.calls)
	}
}

func TestRunChecksDockerBeforeRecreatingRuntime(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	stale := filepath.Join(root, ".segmentstream", "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "Cannot connect to Docker daemon", err: errors.New("docker info failed")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	if !strings.Contains(err.Error(), "Docker is installed") {
		t.Fatalf("error = %v, want Docker Engine message", err)
	}
	assertFileExists(t, stale)
}

func TestRunFailsWhenDockerCLIIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{lookPathErr: exec.ErrNotFound}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	if !strings.Contains(err.Error(), "Docker is required to run SegmentStream locally") {
		t.Fatalf("error = %v, want Docker missing message", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("docker commands were run even though docker is missing: %v", runner.calls)
	}
	assertFileMissing(t, filepath.Join(root, ".segmentstream"))
}

func TestRunFailsWhenDockerEngineIsUnavailable(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "Client:\n Version: 28.3.2\n Plugins:\n  compose: Docker Compose\nServer:\nerror during connect: open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file specified.", err: errors.New("exit status 1")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"Docker is installed",
		"segmentstream run",
		"error during connect",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	for _, notWant := range []string{
		"Plugins:",
		"Docker output:",
	} {
		if strings.Contains(err.Error(), notWant) {
			t.Fatalf("error = %v, did not want noisy detail %q", err, notWant)
		}
	}
	assertFileMissing(t, filepath.Join(root, ".segmentstream"))
}

func TestRunFailsWhenDockerComposeV2IsUnavailable(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{output: "docker: 'compose' is not a docker command", err: errors.New("exit status 1")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"Docker Compose V2 is required",
		"docker: 'compose' is not a docker command",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	assertFileMissing(t, filepath.Join(root, ".segmentstream"))
}

func TestRunPreparesRuntimeAndStartsDockerCompose(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "Docker Compose version v2.32.0"},
			{output: "compose started noisily"},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, ".segmentstream", "docker-compose.yml"))
	if len(runner.calls) != 3 {
		t.Fatalf("docker calls = %v, want 3 calls", runner.calls)
	}
	assertCommand(t, runner.calls[0], "docker", []string{"info", "--format", "{{json .ServerVersion}}"}, "")
	assertCommand(t, runner.calls[1], "docker", []string{"compose", "version"}, "")
	assertCommand(t, runner.calls[2], "docker", []string{"compose", "up", "-d", "--build"}, filepath.Join(root, ".segmentstream"))

	got := out.String()
	for _, want := range []string{
		"Checking Docker...",
		"Preparing .segmentstream runtime...",
		"Starting SegmentStream runtime...",
		"First start can take a few minutes",
		"Started SegmentStream runtime at http://localhost:3000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run output = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "compose started noisily") {
		t.Fatalf("run output = %q, want compose output suppressed", got)
	}
}

func TestRunShowsProgressWhileDockerComposeRuns(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	oldInterval := composeProgressInterval
	composeProgressInterval = time.Millisecond
	t.Cleanup(func() {
		composeProgressInterval = oldInterval
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{delay: 25 * time.Millisecond},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Still starting SegmentStream runtime...") {
		t.Fatalf("run output = %q, want progress message", got)
	}
}

func TestRunIncludesComposeOutputOnFailure(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{output: "failed to solve image", err: errors.New("exit status 1")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, runner)
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"Docker Compose failed to start the SegmentStream runtime",
		"failed to solve image",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

type stubCommandResult struct {
	output string
	err    error
	delay  time.Duration
}

type stubCommandRunner struct {
	lookPathErr error
	lookups     []string
	calls       []commandInvocation
	results     []stubCommandResult
}

func (r *stubCommandRunner) LookPath(file string) (string, error) {
	r.lookups = append(r.lookups, file)
	if r.lookPathErr != nil {
		return "", r.lookPathErr
	}
	return file, nil
}

func (r *stubCommandRunner) Run(ctx context.Context, invocation commandInvocation) (string, error) {
	r.calls = append(r.calls, invocation)
	if len(r.results) == 0 {
		return "", nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	if result.delay > 0 {
		timer := time.NewTimer(result.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return result.output, result.err
}

func writeValidConfig(t *testing.T, root string) {
	t.Helper()
	config := `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`
	if err := os.WriteFile(filepath.Join(root, "segmentstream.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCommand(t *testing.T, got commandInvocation, name string, args []string, dir string) {
	t.Helper()
	if got.Name != name {
		t.Fatalf("command name = %q, want %q", got.Name, name)
	}
	if strings.Join(got.Args, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("command args = %v, want %v", got.Args, args)
	}
	if filepath.Clean(got.Dir) != filepath.Clean(dir) {
		t.Fatalf("command dir = %q, want %q", got.Dir, dir)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing, stat error = %v", path, err)
	}
}
