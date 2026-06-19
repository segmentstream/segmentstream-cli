package googleoauth

import (
	"testing"
)

func TestDefaultConfigReadsEnvironment(t *testing.T) {
	t.Setenv(EnvClientID, "env-client-id")
	t.Setenv(EnvClientSecret, "env-client-secret")

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig failed: %v", err)
	}
	if config.ClientID != "env-client-id" || config.ClientSecret != "env-client-secret" {
		t.Fatalf("config = %+v, want env client credentials", config)
	}
	if len(config.Scopes) != 1 || config.Scopes[0] != "https://www.googleapis.com/auth/bigquery" {
		t.Fatalf("scopes = %+v, want BigQuery scope", config.Scopes)
	}
}

func TestDefaultConfigRequiresClientCredentials(t *testing.T) {
	t.Setenv(EnvClientID, "")
	t.Setenv(EnvClientSecret, "")

	if _, err := DefaultConfig(); err == nil {
		t.Fatal("expected missing client credentials error")
	}
}
