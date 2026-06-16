package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"segmentstream ",
		"commit: ",
		"date: ",
		"os/arch: ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestAuthCommandIncludesBigQuery(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"auth", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth help failed: %v", err)
	}
	if !strings.Contains(out.String(), "bigquery") {
		t.Fatalf("auth help %q does not include bigquery", out.String())
	}
}

func TestAuthBigQueryCommandRunsAuthenticator(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	authenticator := &fakeBigQueryAuthenticator{path: "/tmp/google.json"}

	cmd := newRootCommand(&out, &errOut, cliOptions{
		NewBigQueryAuthenticator: func(io.Writer, io.Writer) bigQueryAuthenticator {
			return authenticator
		},
	})
	cmd.SetArgs([]string{"auth", "bigquery"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth bigquery failed: %v", err)
	}
	if !authenticator.called {
		t.Fatal("authenticator was not called")
	}
}

type fakeBigQueryAuthenticator struct {
	called bool
	path   string
	err    error
}

func (authenticator *fakeBigQueryAuthenticator) AuthenticateBigQuery(context.Context) (string, error) {
	authenticator.called = true
	return authenticator.path, authenticator.err
}
