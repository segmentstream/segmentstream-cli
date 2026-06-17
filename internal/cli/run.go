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

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/projectruntime"
	"github.com/spf13/cobra"
)

const runtimeURL = "http://localhost:3000"

var composeProgressInterval = 15 * time.Second

func newRunCommand(out io.Writer, runner commandRunner) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run SegmentStream analytics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}
			return runAnalytics(cmd.Context(), projectRoot, out, runner)
		},
	}
}

func runAnalytics(ctx context.Context, projectRoot string, out io.Writer, runner commandRunner) error {
	config, err := project.LoadConfig(projectRoot)
	if err != nil {
		return err
	}

	progress := newRunProgress(out, 4)

	progress.Start("Checking local environment")
	if err := preflightDocker(ctx, runner); err != nil {
		return err
	}
	progress.OK("")

	progress.Start("Preparing project files")
	if err := projectruntime.Prepare(projectRoot, config); err != nil {
		return err
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
		return commandError("SegmentStream failed to start.", output, err)
	}
	progress.OK(fmt.Sprintf("ready at %s", runtimeURL))

	progress.Start("Running analytics models")

	output, err = runWithProgress(ctx, progress, runner, commandInvocation{
		Name: "docker",
		Args: []string{
			"compose", "exec", "-T", "segmentstream",
			"dagster", "job", "execute",
			"-f", "dagster/definitions.py",
			"-j", "segmentstream_materialize_all",
		},
		Dir: runtimeDir,
	}, "Still running analytics models")
	if err != nil {
		return commandError("SegmentStream pipeline failed.", output, err)
	}
	progress.OK("")

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Finished SegmentStream pipeline")
	return nil
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
