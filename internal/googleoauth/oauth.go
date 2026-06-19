package googleoauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	bq "google.golang.org/api/bigquery/v2"
)

const (
	EnvClientID     = "SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_ID"
	EnvClientSecret = "SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_SECRET"

	TokenURL = "https://oauth2.googleapis.com/token"
)

var (
	desktopClientID     string
	desktopClientSecret string
)

type BrowserOpener func(string) error

type Config struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
	OpenBrowser  BrowserOpener
}

func Login(ctx context.Context, out io.Writer) (credentials.GoogleOAuthCredential, error) {
	config, err := DefaultConfig()
	if err != nil {
		return credentials.GoogleOAuthCredential{}, err
	}
	return LoginWithConfig(ctx, out, config)
}

func DefaultConfig() (Config, error) {
	clientID := firstNonEmpty(desktopClientID, os.Getenv(EnvClientID))
	clientSecret := firstNonEmpty(desktopClientSecret, os.Getenv(EnvClientSecret))
	if clientID == "" || clientSecret == "" {
		return Config{}, fmt.Errorf("Google OAuth desktop client is not configured; set %s and %s", EnvClientID, EnvClientSecret)
	}
	return Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       DefaultScopes(),
		OpenBrowser:  OpenBrowser,
	}, nil
}

func DefaultScopes() []string {
	return []string{bq.BigqueryScope}
}

func LoginWithConfig(ctx context.Context, out io.Writer, config Config) (credentials.GoogleOAuthCredential, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	if config.ClientID == "" || config.ClientSecret == "" {
		return credentials.GoogleOAuthCredential{}, errors.New("Google OAuth client ID and client secret are required")
	}
	if len(config.Scopes) == 0 {
		config.Scopes = DefaultScopes()
	}
	if config.OpenBrowser == nil {
		config.OpenBrowser = OpenBrowser
	}

	state, err := randomState()
	if err != nil {
		return credentials.GoogleOAuthCredential{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return credentials.GoogleOAuthCredential{}, fmt.Errorf("start OAuth callback listener: %w", err)
	}
	defer listener.Close()

	oauthConfig := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scopes:       config.Scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://" + listener.Addr().String() + "/callback",
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			if got := r.URL.Query().Get("state"); got != state {
				http.Error(w, "OAuth state mismatch. Return to the terminal and retry.", http.StatusBadRequest)
				errCh <- errors.New("OAuth state mismatch")
				return
			}
			if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
				description := r.URL.Query().Get("error_description")
				http.Error(w, "OAuth authorization failed. Return to the terminal for details.", http.StatusBadRequest)
				errCh <- fmt.Errorf("OAuth authorization failed: %s %s", oauthErr, description)
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "OAuth authorization code is missing. Return to the terminal and retry.", http.StatusBadRequest)
				errCh <- errors.New("OAuth authorization code is missing")
				return
			}
			fmt.Fprintln(w, "SegmentStream warehouse login succeeded. You can return to the terminal.")
			codeCh <- code
		}),
	}
	defer server.Shutdown(context.Background())
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	authURL := oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	)
	fmt.Fprintf(out, "Opening browser for SegmentStream warehouse login.\n")
	fmt.Fprintf(out, "If the browser does not open, visit:\n%s\n", authURL)
	if err := config.OpenBrowser(authURL); err != nil {
		fmt.Fprintf(out, "Could not open browser automatically: %v\n", err)
	}

	select {
	case <-ctx.Done():
		return credentials.GoogleOAuthCredential{}, ctx.Err()
	case err := <-errCh:
		return credentials.GoogleOAuthCredential{}, err
	case code := <-codeCh:
		token, err := oauthConfig.Exchange(ctx, code)
		if err != nil {
			return credentials.GoogleOAuthCredential{}, fmt.Errorf("exchange OAuth authorization code: %w", err)
		}
		if strings.TrimSpace(token.RefreshToken) == "" {
			return credentials.GoogleOAuthCredential{}, errors.New("Google did not return a refresh token; revoke prior SegmentStream CLI access in your Google account and retry")
		}
		return credentials.GoogleOAuthCredential{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			RefreshToken: token.RefreshToken,
			TokenURI:     TokenURL,
			Scopes:       config.Scopes,
		}, nil
	}
}

func OpenBrowser(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("refusing to open non-http URL %q", rawURL)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}

func randomState() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate OAuth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
