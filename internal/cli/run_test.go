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

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/dagster"
)

func TestRunFailsWhenConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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

func TestRunPreparesRuntimeStartsDockerComposeAndRunsMaterialization(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withCurrentTime(t, time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC))
	client := withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
		launchBackfillID: "backfill-1",
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "Docker Compose version v2.32.0"},
			{output: "compose started noisily"},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
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
	assertCommand(t, runner.calls[2], "docker", []string{"compose", "up", "-d", "--build", "--force-recreate"}, filepath.Join(root, ".segmentstream"))
	assertStringSlicesEqual(t, client.calls, []string{"waitReady", "materializableAssets", "launchBackfill", "waitBackfill"})
	if client.baseURL != runtimeURL {
		t.Fatalf("Dagster client base URL = %q, want %q", client.baseURL, runtimeURL)
	}
	if client.launchRange.StartDate != "2026-05-19" ||
		client.launchRange.EndInclusiveDate != "2026-06-17" {
		t.Fatalf("launch range = %+v, want default 30-day range", client.launchRange)
	}
	if len(client.launchAssets) != 1 ||
		strings.Join(client.launchAssets[0].Key, "/") != "events" ||
		!client.launchAssets[0].IsPartitioned {
		t.Fatalf("launch assets = %+v, want partitioned events asset", client.launchAssets)
	}
	if client.waitBackfillID != "backfill-1" {
		t.Fatalf("wait backfill id = %q, want backfill-1", client.waitBackfillID)
	}

	got := out.String()
	for _, want := range []string{
		"[1/4] Checking local environment",
		"      OK",
		"[2/4] Preparing project files",
		"[3/4] Starting SegmentStream",
		"First start can take a few minutes",
		"      OK - ready at http://localhost:3000",
		"[4/4] Running pipeline",
		"Processing 2026-05-19 through 2026-06-17",
		"Finished SegmentStream pipeline",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("run output = %q, want %q", got, want)
		}
	}
	for _, notWant := range []string{
		"compose started noisily",
		"Docker",
		"Docker Compose",
		"Dagster",
	} {
		if strings.Contains(got, notWant) {
			t.Fatalf("run output = %q, want command output suppressed", got)
		}
	}
}

func TestRunJSONWritesProgressToStderrAndResultToStdout(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withCurrentTime(t, time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC))
	client := withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
		launchBackfillID: "backfill-1",
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "Docker Compose version v2.32.0"},
			{output: "compose started noisily"},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	var result runResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "run" || response.Status != string(cliresult.StatusOK) {
		t.Fatalf("response = %+v, want successful run response", response)
	}
	if result.Status != "finished" ||
		result.StartDate != "2026-05-19" ||
		result.EndInclusiveDate != "2026-06-17" ||
		result.Assets != 1 {
		t.Fatalf("run result = %+v, want finished one-asset run", result)
	}
	if strings.Contains(out.String(), "[1/4]") || !strings.Contains(errOut.String(), "[1/4] Checking local environment") {
		t.Fatalf("stdout=%q stderr=%q, want progress on stderr only", out.String(), errOut.String())
	}
	assertStringSlicesEqual(t, client.calls, []string{"waitReady", "materializableAssets", "launchBackfill", "waitBackfill"})
}

func TestRunAcceptsStartDate(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withCurrentTime(t, time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC))
	client := withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run", "--start-date", "2026-06-01"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	if client.launchRange.StartDate != "2026-06-01" ||
		client.launchRange.EndInclusiveDate != "2026-06-17" {
		t.Fatalf("launch range = %+v, want custom date range", client.launchRange)
	}
	if !strings.Contains(out.String(), "Processing 2026-06-01 through 2026-06-17") {
		t.Fatalf("run output = %q, want custom date range", out.String())
	}
}

func TestRunRejectsInvalidStartDateBeforeDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run", "--start-date", "June 1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	if !strings.Contains(err.Error(), `invalid --start-date "June 1"; use YYYY-MM-DD`) {
		t.Fatalf("error = %v, want invalid start date message", err)
	}
	if len(runner.lookups) != 0 || len(runner.calls) != 0 {
		t.Fatalf("docker was called before start date was validated: lookups=%v calls=%v", runner.lookups, runner.calls)
	}
}

func TestRunRejectsFutureStartDateBeforeDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withCurrentTime(t, time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC))

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run", "--start-date", "2026-06-18"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	if !strings.Contains(err.Error(), "--start-date 2026-06-18 is after current UTC date 2026-06-17") {
		t.Fatalf("error = %v, want future start date message", err)
	}
	if len(runner.lookups) != 0 || len(runner.calls) != 0 {
		t.Fatalf("docker was called before start date was validated: lookups=%v calls=%v", runner.lookups, runner.calls)
	}
}

func TestRunFailsWhenBigQueryAuthIsMissingBeforeDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
sources:
  - name: ga4
    path: ./sources/ga4
`)
	withRunAuthHome(t, filepath.Join(root, "home"))

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream run sanity check failed",
		"BigQuery authentication",
		"segmentstream warehouse auth --service-account-key",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if len(runner.lookups) != 0 || len(runner.calls) != 0 {
		t.Fatalf("docker was called before auth was checked: lookups=%v calls=%v", runner.lookups, runner.calls)
	}
	assertFileMissing(t, filepath.Join(root, ".segmentstream"))
}

func TestRunFailsWhenNoSourcesConfiguredBeforeDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`)
	home := filepath.Join(root, "home")
	withRunAuthHome(t, home)
	writeBigQueryAuth(t, home)

	runner := &stubCommandRunner{}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream run sanity check failed",
		"at least one source",
		"segmentstream source scaffold <name> --type events",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if len(runner.lookups) != 0 || len(runner.calls) != 0 {
		t.Fatalf("docker was called before sources were checked: lookups=%v calls=%v", runner.lookups, runner.calls)
	}
	assertFileMissing(t, filepath.Join(root, ".segmentstream"))
}

func TestRunShowsProgressWhileDockerComposeRuns(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
	})

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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Still starting SegmentStream...") {
		t.Fatalf("run output = %q, want progress message", got)
	}
}

func TestRunShowsProgressWhilePipelineRuns(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
		waitBackfillDelay: 25 * time.Millisecond,
	})

	oldInterval := composeProgressInterval
	composeProgressInterval = time.Millisecond
	t.Cleanup(func() {
		composeProgressInterval = oldInterval
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Still running pipeline...") {
		t.Fatalf("run output = %q, want pipeline progress message", got)
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
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream failed to start",
		"failed to solve image",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestRunFailsClearlyWhenSegmentStreamIsNotReady(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withDagsterClient(t, &stubDagsterClient{
		waitReadyErr: errors.New("service did not answer before timeout"),
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream failed to start",
		"service did not answer before timeout",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestRunFailsClearlyWhenBackfillLaunchFails(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
		launchErr: errors.New("partition keys could not be found"),
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream pipeline failed",
		"partition keys could not be found",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestRunSucceedsWhenThereAreNoMaterializableAssets(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	client := withDagsterClient(t, &stubDagsterClient{})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("run command failed: %v", err)
	}
	assertStringSlicesEqual(t, client.calls, []string{"waitReady", "materializableAssets"})
	if !strings.Contains(out.String(), "Nothing to run") {
		t.Fatalf("run output = %q, want nothing-to-run message", out.String())
	}
}

func TestRunIncludesPipelineFailureDetail(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	withDagsterClient(t, &stubDagsterClient{
		assets: []dagster.AssetNode{
			{Key: []string{"events"}, IsPartitioned: true},
		},
		waitBackfillErr: errors.New("asset backfill failed"),
	})

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{},
			{},
			{},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected run to fail")
	}
	for _, want := range []string{
		"SegmentStream pipeline failed",
		"asset backfill failed",
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

type stubDagsterClient struct {
	baseURL           string
	calls             []string
	assets            []dagster.AssetNode
	waitReadyErr      error
	assetsErr         error
	launchErr         error
	waitBackfillErr   error
	launchBackfillID  string
	launchAssets      []dagster.AssetNode
	launchRange       dagster.DateRange
	waitBackfillID    string
	waitReadyDelay    time.Duration
	waitBackfillDelay time.Duration
}

func (c *stubDagsterClient) WaitUntilReady(ctx context.Context) error {
	c.calls = append(c.calls, "waitReady")
	if err := waitForStubDelay(ctx, c.waitReadyDelay); err != nil {
		return err
	}
	return c.waitReadyErr
}

func (c *stubDagsterClient) MaterializableAssets(ctx context.Context) ([]dagster.AssetNode, error) {
	c.calls = append(c.calls, "materializableAssets")
	if c.assetsErr != nil {
		return nil, c.assetsErr
	}
	return append([]dagster.AssetNode(nil), c.assets...), nil
}

func (c *stubDagsterClient) LaunchBackfill(ctx context.Context, assets []dagster.AssetNode, runRange dagster.DateRange) (string, error) {
	c.calls = append(c.calls, "launchBackfill")
	if c.launchErr != nil {
		return "", c.launchErr
	}
	c.launchAssets = append([]dagster.AssetNode(nil), assets...)
	c.launchRange = runRange
	if c.launchBackfillID != "" {
		return c.launchBackfillID, nil
	}
	return "backfill-1", nil
}

func (c *stubDagsterClient) WaitForBackfill(ctx context.Context, backfillID string) error {
	c.calls = append(c.calls, "waitBackfill")
	c.waitBackfillID = backfillID
	if err := waitForStubDelay(ctx, c.waitBackfillDelay); err != nil {
		return err
	}
	return c.waitBackfillErr
}

func waitForStubDelay(ctx context.Context, delay time.Duration) error {
	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func withDagsterClient(t *testing.T, client *stubDagsterClient) *stubDagsterClient {
	t.Helper()
	previous := newDagsterClient
	newDagsterClient = func(baseURL string) dagster.Client {
		client.baseURL = baseURL
		return client
	}
	t.Cleanup(func() {
		newDagsterClient = previous
	})
	return client
}

func writeValidConfig(t *testing.T, root string) {
	t.Helper()
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
sources:
  - name: ga4
    path: ./sources/ga4
`)
	home := filepath.Join(root, "home")
	withRunAuthHome(t, home)
	writeBigQueryAuth(t, home)
}

func writeConfig(t *testing.T, root string, config string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "segmentstream.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withRunAuthHome(t *testing.T, home string) {
	t.Helper()
	previous := newRunCredentialStore
	newRunCredentialStore = func() credentials.Store {
		return credentials.Store{HomeDir: home}
	}
	t.Cleanup(func() {
		newRunCredentialStore = previous
	})
}

func writeBigQueryAuth(t *testing.T, home string) {
	t.Helper()
	credentialsPath, err := (credentials.Store{HomeDir: home}).CredentialPath("bigquery", "production-bigquery")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(credentialsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialsPath, []byte(`{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n"}`), 0o600); err != nil {
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
	if comparablePath(t, got.Dir) != comparablePath(t, dir) {
		t.Fatalf("command dir = %q, want %q", got.Dir, dir)
	}
}

func comparablePath(t *testing.T, path string) string {
	t.Helper()
	if path == "" {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("slice = %v, want %v", got, want)
	}
}

func withCurrentTime(t *testing.T, value time.Time) {
	t.Helper()
	previous := currentTime
	currentTime = func() time.Time { return value }
	t.Cleanup(func() {
		currentTime = previous
	})
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing, stat error = %v", path, err)
	}
}
