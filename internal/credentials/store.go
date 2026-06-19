package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	segmentStreamDirName = ".segmentstream"
	bigQueryDirName      = "bigquery"
	accessMarkerSuffix   = ".access.json"
)

type Store struct {
	HomeDir string
}

type AccessMarker struct {
	Project   string `json:"project"`
	Dataset   string `json:"dataset"`
	Location  string `json:"location"`
	CheckedAt string `json:"checked_at"`
}

type GoogleOAuthCredential struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RefreshToken string   `json:"refresh_token"`
	TokenURI     string   `json:"token_uri,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

func (store Store) BigQueryCredentialPath(name string) (string, error) {
	if err := validateCredentialName(name); err != nil {
		return "", err
	}
	root, err := store.bigQueryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name+".json"), nil
}

func (store Store) HasBigQueryCredential(name string) (bool, error) {
	path, err := store.BigQueryCredentialPath(name)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check BigQuery credential %q: %w", name, err)
	}
	if info.IsDir() {
		return false, fmt.Errorf("BigQuery credential path %s is a directory", path)
	}
	return true, nil
}

func (store Store) SaveServiceAccountKey(name, sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("--service-account-key is required")
	}
	if err := validateCredentialName(name); err != nil {
		return "", err
	}

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read service account key: %w", err)
	}
	if err := validateServiceAccountJSON(data); err != nil {
		return "", err
	}

	dir, err := store.bigQueryDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create BigQuery credential directory: %w", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write BigQuery credential: %w", err)
	}
	if err := store.deleteAccessMarker(name); err != nil {
		return "", err
	}
	return path, nil
}

func (store Store) SaveGoogleOAuthCredential(name string, credential GoogleOAuthCredential) (string, error) {
	if err := validateCredentialName(name); err != nil {
		return "", err
	}
	if err := validateGoogleOAuthCredential(credential); err != nil {
		return "", err
	}

	payload := struct {
		Type         string   `json:"type"`
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RefreshToken string   `json:"refresh_token"`
		TokenURI     string   `json:"token_uri"`
		Scopes       []string `json:"scopes,omitempty"`
	}{
		Type:         "authorized_user",
		ClientID:     strings.TrimSpace(credential.ClientID),
		ClientSecret: strings.TrimSpace(credential.ClientSecret),
		RefreshToken: strings.TrimSpace(credential.RefreshToken),
		TokenURI:     strings.TrimSpace(credential.TokenURI),
		Scopes:       credential.Scopes,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal Google OAuth credential: %w", err)
	}

	dir, err := store.bigQueryDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create BigQuery credential directory: %w", err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write Google OAuth credential: %w", err)
	}
	if err := store.deleteAccessMarker(name); err != nil {
		return "", err
	}
	return path, nil
}

func (store Store) SaveAccessMarker(name, project, dataset, location string) error {
	path, err := store.accessMarkerPath(name)
	if err != nil {
		return err
	}
	marker := AccessMarker{
		Project:   project,
		Dataset:   dataset,
		Location:  location,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal BigQuery access marker: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create BigQuery credential directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write BigQuery access marker: %w", err)
	}
	return nil
}

func (store Store) HasMatchingAccessMarker(name, project, dataset, location string) (bool, error) {
	path, err := store.accessMarkerPath(name)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read BigQuery access marker: %w", err)
	}
	var marker AccessMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return false, nil
	}
	return marker.Project == project &&
		marker.Dataset == dataset &&
		strings.EqualFold(marker.Location, location), nil
}

func (store Store) accessMarkerPath(name string) (string, error) {
	if err := validateCredentialName(name); err != nil {
		return "", err
	}
	root, err := store.bigQueryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name+accessMarkerSuffix), nil
}

func (store Store) deleteAccessMarker(name string) error {
	path, err := store.accessMarkerPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove stale BigQuery access marker: %w", err)
	}
	return nil
}

func (store Store) bigQueryDir() (string, error) {
	home, err := store.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, segmentStreamDirName, bigQueryDirName), nil
}

func (store Store) homeDir() (string, error) {
	if store.HomeDir != "" {
		return store.HomeDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("find home directory: home directory is empty")
	}
	return home, nil
}

func validateCredentialName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("credential name is required")
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' ||
			char == '_' ||
			char == '.' {
			continue
		}
		return fmt.Errorf("invalid credential name %q; use only letters, numbers, dots, hyphens, and underscores", name)
	}
	return nil
}

func validateServiceAccountJSON(data []byte) error {
	var payload struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("service account key is not valid JSON: %w", err)
	}
	if payload.Type != "service_account" {
		return fmt.Errorf("service account key has type %q, want service_account", payload.Type)
	}
	if strings.TrimSpace(payload.ClientEmail) == "" || strings.TrimSpace(payload.PrivateKey) == "" {
		return errors.New("service account key is missing client_email or private_key")
	}
	return nil
}

func validateGoogleOAuthCredential(credential GoogleOAuthCredential) error {
	if strings.TrimSpace(credential.ClientID) == "" {
		return errors.New("Google OAuth client_id is required")
	}
	if strings.TrimSpace(credential.ClientSecret) == "" {
		return errors.New("Google OAuth client_secret is required")
	}
	if strings.TrimSpace(credential.RefreshToken) == "" {
		return errors.New("Google OAuth refresh_token is required")
	}
	if strings.TrimSpace(credential.TokenURI) == "" {
		return errors.New("Google OAuth token_uri is required")
	}
	return nil
}
