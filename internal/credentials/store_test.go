package credentials

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBigQueryCredentialPath(t *testing.T) {
	home := t.TempDir()
	path, err := (Store{HomeDir: home}).BigQueryCredentialPath("default-bigquery")
	if err != nil {
		t.Fatalf("BigQueryCredentialPath failed: %v", err)
	}
	want := filepath.Join(home, ".segmentstream", "bigquery", "default-bigquery.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestSaveServiceAccountKey(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "key.json")
	data := []byte(`{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"secret"}`)
	if err := os.WriteFile(keyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := (Store{HomeDir: filepath.Join(root, "home")}).SaveServiceAccountKey("default-bigquery", keyPath)
	if err != nil {
		t.Fatalf("SaveServiceAccountKey failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("stored key = %s, want source key", string(got))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSaveServiceAccountKeyClearsAccessMarker(t *testing.T) {
	root := t.TempDir()
	store := Store{HomeDir: filepath.Join(root, "home")}
	if err := store.SaveAccessMarker("default-bigquery", "example-project", "segmentstream", "EU"); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	matches, err := store.HasMatchingAccessMarker("default-bigquery", "example-project", "segmentstream", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("expected marker before replacing credential")
	}

	keyPath := filepath.Join(root, "key.json")
	data := []byte(`{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"secret"}`)
	if err := os.WriteFile(keyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveServiceAccountKey("default-bigquery", keyPath); err != nil {
		t.Fatalf("SaveServiceAccountKey failed: %v", err)
	}

	matches, err = store.HasMatchingAccessMarker("default-bigquery", "example-project", "segmentstream", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("expected marker to be cleared after replacing credential")
	}
}

func TestSaveGoogleOAuthCredential(t *testing.T) {
	root := t.TempDir()
	store := Store{HomeDir: filepath.Join(root, "home")}

	path, err := store.SaveGoogleOAuthCredential("default-bigquery", GoogleOAuthCredential{
		ClientID:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
		TokenURI:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/bigquery"},
	})
	if err != nil {
		t.Fatalf("SaveGoogleOAuthCredential failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"type": "authorized_user"`,
		`"client_id": "client-id.apps.googleusercontent.com"`,
		`"client_secret": "client-secret"`,
		`"refresh_token": "refresh-token"`,
		`"token_uri": "https://oauth2.googleapis.com/token"`,
		`"https://www.googleapis.com/auth/bigquery"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("stored OAuth credential = %s, want %q", string(data), want)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSaveGoogleOAuthCredentialClearsAccessMarker(t *testing.T) {
	root := t.TempDir()
	store := Store{HomeDir: filepath.Join(root, "home")}
	if err := store.SaveAccessMarker("default-bigquery", "example-project", "segmentstream", "EU"); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}

	if _, err := store.SaveGoogleOAuthCredential("default-bigquery", GoogleOAuthCredential{
		ClientID:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
		TokenURI:     "https://oauth2.googleapis.com/token",
	}); err != nil {
		t.Fatalf("SaveGoogleOAuthCredential failed: %v", err)
	}

	matches, err := store.HasMatchingAccessMarker("default-bigquery", "example-project", "segmentstream", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("expected marker to be cleared after replacing credential")
	}
}

func TestSaveGoogleOAuthCredentialRejectsMissingRefreshToken(t *testing.T) {
	root := t.TempDir()

	_, err := (Store{HomeDir: filepath.Join(root, "home")}).SaveGoogleOAuthCredential("default-bigquery", GoogleOAuthCredential{
		ClientID:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		TokenURI:     "https://oauth2.googleapis.com/token",
	})
	if err == nil {
		t.Fatal("expected missing refresh token error")
	}
}

func TestSaveServiceAccountKeyRejectsWrongType(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "key.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"authorized_user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := (Store{HomeDir: filepath.Join(root, "home")}).SaveServiceAccountKey("default-bigquery", keyPath)
	if err == nil {
		t.Fatal("expected wrong credential type error")
	}
}

func TestAccessMarkerMatchesWarehouseConfig(t *testing.T) {
	home := t.TempDir()
	store := Store{HomeDir: home}
	if err := store.SaveAccessMarker("default-bigquery", "example-project", "segmentstream", "EU"); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	matches, err := store.HasMatchingAccessMarker("default-bigquery", "example-project", "segmentstream", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("expected marker to match")
	}
	matches, err = store.HasMatchingAccessMarker("default-bigquery", "example-project", "other", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("expected marker mismatch for different dataset")
	}
}
