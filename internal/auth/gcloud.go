package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	CloudPlatformScope   = "https://www.googleapis.com/auth/cloud-platform"
	BigQueryScope        = "https://www.googleapis.com/auth/bigquery"
	gcloudConfigDirName  = "gcloud"
	gcloudADCFileName    = "application_default_credentials.json"
	gcloudConfigEnv      = "CLOUDSDK_CONFIG"
	defaultGCloudCommand = "gcloud"
)

var bigQueryAuthScopes = []string{CloudPlatformScope, BigQueryScope}

type Store struct {
	HomeDir string
}

type GCloudAuthenticator struct {
	Store   Store
	Command string
	Runner  CommandRunner
	Out     io.Writer
	ErrOut  io.Writer
}

type CommandRunner func(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error

func NewGCloudAuthenticator(out, errOut io.Writer) GCloudAuthenticator {
	return GCloudAuthenticator{
		Runner: defaultCommandRunner,
		Out:    out,
		ErrOut: errOut,
	}
}

func (authenticator GCloudAuthenticator) AuthenticateBigQuery(ctx context.Context) (string, error) {
	configDir, err := authenticator.Store.GCloudConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", fmt.Errorf("create gcloud config directory: %w", err)
	}

	credentialsPath, err := authenticator.Store.GCloudADCPath()
	if err != nil {
		return "", err
	}

	out := authenticator.Out
	if out == nil {
		out = io.Discard
	}
	errOut := authenticator.ErrOut
	if errOut == nil {
		errOut = io.Discard
	}

	command := authenticator.Command
	if command == "" {
		command = defaultGCloudCommand
	}
	runner := authenticator.Runner
	if runner == nil {
		runner = defaultCommandRunner
	}

	fmt.Fprintln(out, "Opening Google authentication with gcloud.")
	err = runner(ctx, command, []string{
		"auth",
		"application-default",
		"login",
		"--scopes=" + strings.Join(bigQueryAuthScopes, ","),
	}, []string{gcloudConfigEnv + "=" + configDir}, out, errOut)
	if err != nil {
		return "", explainGCloudError(err)
	}

	if _, err := os.Stat(credentialsPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("gcloud completed but ADC credentials were not found at %s", credentialsPath)
		}
		return "", fmt.Errorf("check ADC credentials: %w", err)
	}

	fmt.Fprintf(out, "Saved BigQuery credentials to %s\n", credentialsPath)
	return credentialsPath, nil
}

func (store Store) GCloudConfigDir() (string, error) {
	home, err := store.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".segmentstream", gcloudConfigDirName), nil
}

func (store Store) GCloudADCPath() (string, error) {
	configDir, err := store.GCloudConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, gcloudADCFileName), nil
}

func (store Store) homeDir() (string, error) {
	if store.HomeDir != "" {
		return store.HomeDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return home, nil
}

func defaultCommandRunner(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func explainGCloudError(err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return errors.New("gcloud was not found on PATH; install Google Cloud SDK before running segmentstream auth add bigquery")
	}
	return fmt.Errorf("run gcloud application default login: %w", err)
}
