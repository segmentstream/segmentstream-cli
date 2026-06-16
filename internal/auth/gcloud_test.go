package auth

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreGCloudADCPath(t *testing.T) {
	home := t.TempDir()
	got, err := (Store{HomeDir: home}).GCloudADCPath()
	if err != nil {
		t.Fatalf("GCloudADCPath failed: %v", err)
	}

	want := filepath.Join(home, ".segmentstream", "gcloud", "application_default_credentials.json")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestAuthenticateBigQueryRunsGCloudWithIsolatedConfig(t *testing.T) {
	home := t.TempDir()
	var gotCommand string
	var gotArgs []string
	var gotEnv []string

	authenticator := GCloudAuthenticator{
		Store:   Store{HomeDir: home},
		Command: "gcloud-test",
		Runner: func(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error {
			gotCommand = command
			gotArgs = append([]string(nil), args...)
			gotEnv = append([]string(nil), env...)

			credentialsPath, err := (Store{HomeDir: home}).GCloudADCPath()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(credentialsPath), 0o700); err != nil {
				return err
			}
			return os.WriteFile(credentialsPath, []byte(`{"type":"authorized_user"}`), 0o600)
		},
	}

	path, err := authenticator.AuthenticateBigQuery(context.Background())
	if err != nil {
		t.Fatalf("AuthenticateBigQuery failed: %v", err)
	}

	wantPath := filepath.Join(home, ".segmentstream", "gcloud", "application_default_credentials.json")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	if gotCommand != "gcloud-test" {
		t.Fatalf("command = %q, want gcloud-test", gotCommand)
	}
	wantArgs := []string{"auth", "application-default", "login", "--scopes=" + strings.Join(bigQueryAuthScopes, ",")}
	if strings.Join(gotArgs, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", gotArgs, wantArgs)
	}
	wantEnv := gcloudConfigEnv + "=" + filepath.Join(home, ".segmentstream", "gcloud")
	if len(gotEnv) != 1 || gotEnv[0] != wantEnv {
		t.Fatalf("env = %#v, want %#v", gotEnv, []string{wantEnv})
	}
}

func TestAuthenticateBigQueryRequiresGeneratedCredentials(t *testing.T) {
	authenticator := GCloudAuthenticator{
		Store: Store{HomeDir: t.TempDir()},
		Runner: func(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error {
			return nil
		},
	}

	_, err := authenticator.AuthenticateBigQuery(context.Background())
	if err == nil {
		t.Fatal("expected missing credentials error")
	}
	if !strings.Contains(err.Error(), "ADC credentials were not found") {
		t.Fatalf("error = %v, want missing credentials", err)
	}
}

func TestAuthenticateBigQueryExplainsMissingGCloud(t *testing.T) {
	authenticator := GCloudAuthenticator{
		Store: Store{HomeDir: t.TempDir()},
		Runner: func(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error {
			return &exec.Error{Name: command, Err: exec.ErrNotFound}
		},
	}

	_, err := authenticator.AuthenticateBigQuery(context.Background())
	if err == nil {
		t.Fatal("expected gcloud missing error")
	}
	if !strings.Contains(err.Error(), "gcloud was not found") {
		t.Fatalf("error = %v, want gcloud guidance", err)
	}
}

func TestAuthenticateBigQueryWrapsRunnerError(t *testing.T) {
	authenticator := GCloudAuthenticator{
		Store: Store{HomeDir: t.TempDir()},
		Runner: func(ctx context.Context, command string, args []string, env []string, stdout, stderr io.Writer) error {
			return errors.New("boom")
		},
	}

	_, err := authenticator.AuthenticateBigQuery(context.Background())
	if err == nil {
		t.Fatal("expected runner error")
	}
	if !strings.Contains(err.Error(), "run gcloud application default login") {
		t.Fatalf("error = %v, want wrapped runner error", err)
	}
}
