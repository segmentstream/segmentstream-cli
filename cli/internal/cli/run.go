package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/dagster"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/projectruntime"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	"github.com/spf13/cobra"
)

const runtimeURL = "http://localhost:3000"
const defaultRunDays = 30

var composeProgressInterval = 15 * time.Second
var currentTime = func() time.Time { return time.Now().UTC() }
var newDagsterClient = dagster.NewClient
var newRunCredentialStore = func() credentials.Store { return credentials.Store{} }

type runOptions struct {
	StartDate string
}

type runResult struct {
	Status           string `json:"status"`
	RuntimeURL       string `json:"runtime_url"`
	StartDate        string `json:"start_date"`
	EndInclusiveDate string `json:"end_inclusive_date"`
	Assets           int    `json:"assets"`
}

func newRunCommand(out, errOut io.Writer, commandContext structuredCommandContext, runner commandRunner, registry warehouse.Registry, credentialStore credentials.Store) *cobra.Command {
	options := runOptions{}
	cmd := newStructuredCommand(out, errOut, commandContext, structuredCommandSpec{
		Use:     "run",
		Short:   "Run SegmentStream analytics",
		Args:    cobra.NoArgs,
		Command: "run",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		progressOut := out
		if commandContext.Output != nil && commandContext.Output.JSON && errOut != nil {
			progressOut = errOut
		}
		result, err := runAnalytics(ctx, projectRoot, progressOut, runner, registry, credentialStore, options)
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("run", result), nil
	})
	cmd.Flags().StringVar(&options.StartDate, "start-date", "", "Run from this UTC date in YYYY-MM-DD format; defaults to the last 30 days")
	return cmd
}

func runAnalytics(ctx context.Context, projectRoot string, progressOut io.Writer, runner commandRunner, registry warehouse.Registry, credentialStore credentials.Store, options runOptions) (runResult, error) {
	config, err := project.LoadConfig(projectRoot)
	if err != nil {
		return runResult{}, err
	}
	provider, err := registry.Provider(config.Warehouse.Type)
	if err != nil {
		return runResult{}, err
	}
	runRange, err := resolveRunDateRange(options)
	if err != nil {
		return runResult{}, err
	}
	if err := preflightProjectSanity(config, provider, credentialStore); err != nil {
		return runResult{}, err
	}

	progress := newRunProgress(progressOut, 4)

	progress.Start("Checking local environment")
	if err := projectruntime.ValidateAnalyticsCoreDependency(); err != nil {
		return runResult{}, err
	}
	if err := preflightDocker(ctx, runner); err != nil {
		return runResult{}, err
	}
	progress.OK("")

	progress.Start("Preparing project files")
	if err := projectruntime.Prepare(projectRoot, config, provider); err != nil {
		return runResult{}, err
	}
	progress.OK("")

	runtimeDir := filepath.Join(projectRoot, projectruntime.RuntimeDirName)
	progress.Start("Starting SegmentStream")
	progress.Detail("First start can take a few minutes while SegmentStream sets up the local environment.")

	output, err := runWithProgress(ctx, progress, runner, commandInvocation{
		Name: "docker",
		Args: []string{"compose", "up", "-d", "--build", "--force-recreate"},
		Dir:  runtimeDir,
	}, "Still starting SegmentStream")
	if err != nil {
		return runResult{}, commandError("SegmentStream failed to start.", output, err)
	}

	client := newDagsterClient(runtimeURL)
	if err := runOperationWithProgress(ctx, progress, client.WaitUntilReady, "Still starting SegmentStream"); err != nil {
		return runResult{}, operationError("SegmentStream failed to start.", err)
	}
	progress.OK(fmt.Sprintf("ready at %s", runtimeURL))

	progress.Start("Running pipeline")
	progress.Detail(fmt.Sprintf("Processing %s through %s", runRange.StartDate, runRange.EndInclusiveDate))

	assets, err := client.MaterializableAssets(ctx)
	if err != nil {
		return runResult{}, operationError("SegmentStream pipeline failed.", err)
	}
	if len(assets) == 0 {
		progress.OK("nothing to run")
		return runResult{
			Status:           "nothing_to_run",
			RuntimeURL:       runtimeURL,
			StartDate:        runRange.StartDate,
			EndInclusiveDate: runRange.EndInclusiveDate,
			Assets:           0,
		}, nil
	}

	backfillID, err := client.LaunchBackfill(ctx, assets, dagster.DateRange{
		StartDate:        runRange.StartDate,
		EndInclusiveDate: runRange.EndInclusiveDate,
	})
	if err != nil {
		return runResult{}, operationError("SegmentStream pipeline failed.", err)
	}
	if err := runOperationWithProgress(ctx, progress, func(ctx context.Context) error {
		return client.WaitForBackfill(ctx, backfillID)
	}, "Still running pipeline"); err != nil {
		return runResult{}, operationError("SegmentStream pipeline failed.", err)
	}
	progress.OK("")

	return runResult{
		Status:           "finished",
		RuntimeURL:       runtimeURL,
		StartDate:        runRange.StartDate,
		EndInclusiveDate: runRange.EndInclusiveDate,
		Assets:           len(assets),
	}, nil
}

func (result runResult) HumanDocument() cliresult.Document {
	message := "Finished SegmentStream pipeline"
	if result.Status == "nothing_to_run" {
		message = "Nothing to run"
	}
	return cliresult.Document{
		Blocks: []cliresult.Block{
			{Kind: cliresult.BlockCode, Text: message},
		},
	}
}

func preflightProjectSanity(config project.Config, provider warehouse.Provider, credentialStore credentials.Store) error {
	var failures []string

	if err := preflightWarehouseAuth(config, provider, credentialStore); err != nil {
		failures = append(failures, err.Error())
	}
	if len(config.Sources) == 0 {
		failures = append(failures, "segmentstream.yml must declare at least one events source, one identity_keys source, and one conversion_events source under sources; run segmentstream source contracts, then scaffold and verify all required source contracts")
	}
	if len(failures) > 0 {
		return runSanityError(failures)
	}
	return nil
}

func preflightWarehouseAuth(config project.Config, provider warehouse.Provider, credentialStore credentials.Store) error {
	for _, diagnostic := range provider.ConfigDiagnostics(config.Warehouse) {
		if diagnostic.Message != "" {
			return errors.New(diagnostic.Message)
		}
	}

	credentialsPath, err := provider.CredentialPath(credentialStore, config.Warehouse.Auth)
	if err != nil {
		return fmt.Errorf("check warehouse authentication: %w", err)
	}

	info, err := os.Stat(credentialsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s authentication for warehouse.auth %q was not found at %s; run segmentstream warehouse auth login or segmentstream warehouse auth --service-account-key=<path>", provider.DisplayName(), config.Warehouse.Auth, credentialsPath)
		}
		return fmt.Errorf("check warehouse authentication at %s: %w", credentialsPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s authentication path %s is a directory; run segmentstream warehouse auth login or segmentstream warehouse auth --service-account-key=<path>", provider.DisplayName(), credentialsPath)
	}

	return nil
}

type runSanityError []string

func (err runSanityError) Error() string {
	if len(err) == 1 {
		return "SegmentStream run sanity check failed: " + err[0]
	}

	var message strings.Builder
	message.WriteString("SegmentStream run sanity checks failed:")
	for _, failure := range err {
		fmt.Fprintf(&message, "\n- %s", failure)
	}
	return message.String()
}

type runDateRange struct {
	StartDate        string
	EndInclusiveDate string
}

func resolveRunDateRange(options runOptions) (runDateRange, error) {
	today := utcDate(currentTime())
	start := today.AddDate(0, 0, -(defaultRunDays - 1))
	if strings.TrimSpace(options.StartDate) != "" {
		parsed, err := parseDateFlag(options.StartDate, "--start-date")
		if err != nil {
			return runDateRange{}, err
		}
		start = parsed
	}
	if start.After(today) {
		return runDateRange{}, fmt.Errorf("--start-date %s is after current UTC date %s", formatDate(start), formatDate(today))
	}

	return runDateRange{
		StartDate:        formatDate(start),
		EndInclusiveDate: formatDate(today),
	}, nil
}

func parseDateFlag(value, name string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s %q; use YYYY-MM-DD", name, value)
	}
	return utcDate(parsed), nil
}

func utcDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func formatDate(value time.Time) string {
	return value.UTC().Format("2006-01-02")
}

type runProgress struct {
	out     io.Writer
	total   int
	current int
}

func newRunProgress(out io.Writer, total int) *runProgress {
	return &runProgress{out: out, total: total}
}

func (p *runProgress) Start(message string) {
	p.current++
	fmt.Fprintf(p.out, "[%d/%d] %s\n", p.current, p.total, message)
}

func (p *runProgress) Detail(message string) {
	fmt.Fprintf(p.out, "      %s\n", message)
}

func (p *runProgress) OK(message string) {
	if message == "" {
		fmt.Fprintln(p.out, "      OK")
		return
	}
	fmt.Fprintf(p.out, "      OK - %s\n", message)
}

func (p *runProgress) StillWorking(message string, elapsed time.Duration) {
	fmt.Fprintf(p.out, "      %s... %s elapsed\n", message, formatElapsed(elapsed))
}

func runWithProgress(ctx context.Context, progress *runProgress, runner commandRunner, invocation commandInvocation, progressMessage string) (string, error) {
	type commandResult struct {
		output string
		err    error
	}

	done := make(chan commandResult, 1)
	startedAt := time.Now()
	go func() {
		output, err := runner.Run(ctx, invocation)
		done <- commandResult{output: output, err: err}
	}()

	ticker := time.NewTicker(composeProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case result := <-done:
			return result.output, result.err
		case <-ticker.C:
			progress.StillWorking(progressMessage, time.Since(startedAt))
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func runOperationWithProgress(ctx context.Context, progress *runProgress, operation func(context.Context) error, progressMessage string) error {
	done := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		done <- operation(ctx)
	}()

	ticker := time.NewTicker(composeProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			progress.StillWorking(progressMessage, time.Since(startedAt))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func formatElapsed(duration time.Duration) string {
	duration = duration.Round(time.Second)
	if duration < time.Second {
		return "0s"
	}

	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

func preflightDocker(ctx context.Context, runner commandRunner) error {
	if _, err := runner.LookPath("docker"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("Docker is required to run SegmentStream locally. Install Docker Desktop and make sure docker is on PATH.")
		}
		return fmt.Errorf("check Docker CLI: %w", err)
	}

	if output, err := runner.Run(ctx, commandInvocation{Name: "docker", Args: []string{"info", "--format", "{{json .ServerVersion}}"}}); err != nil {
		return dockerEngineUnavailableError(output, err)
	}

	if output, err := runner.Run(ctx, commandInvocation{Name: "docker", Args: []string{"compose", "version"}}); err != nil {
		return commandError("Docker Compose V2 is required. Install or update Docker Desktop so 'docker compose' is available.", output, err)
	}

	return nil
}

func commandError(message, output string, err error) error {
	output = strings.TrimSpace(output)
	if output != "" {
		return fmt.Errorf("%s\n\nDetails:\n%s", message, output)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return errors.New(message)
}

func operationError(message string, err error) error {
	if err == nil {
		return errors.New(message)
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return errors.New(message)
	}
	return fmt.Errorf("%s\n\nDetails:\n%s", message, detail)
}

func dockerEngineUnavailableError(output string, err error) error {
	message := dockerEngineUnavailableMessage()
	detail := usefulDockerDetail(output, err)
	if detail != "" {
		return fmt.Errorf("%s\n\nDetails:\n%s", message, detail)
	}
	return errors.New(message)
}

func dockerEngineUnavailableMessage() string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return "Docker is installed, but Docker Desktop is not running.\n\nOpen Docker Desktop and wait until it finishes starting, then run:\n  segmentstream run\n\nIf Docker Desktop is already open, restart it and try again."
	}

	return "Docker is installed, but the Docker Engine is not running or this user cannot access it.\n\nStart Docker Desktop or the Docker daemon, then run:\n  segmentstream run\n\nOn Linux, this can also mean the current user cannot access the Docker socket."
}

func usefulDockerDetail(output string, err error) string {
	output = strings.TrimSpace(output)
	if output != "" {
		return conciseDockerOutput(output)
	}
	if err == nil || strings.HasPrefix(err.Error(), "exit status ") {
		return ""
	}
	return err.Error()
}

func conciseDockerOutput(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error during connect") ||
			strings.Contains(lower, "cannot connect") ||
			strings.Contains(lower, "connection refused") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "is the docker daemon running") {
			return line
		}
	}

	if len(lines) <= 4 {
		return strings.Join(lines, "\n")
	}
	return lines[len(lines)-1]
}
