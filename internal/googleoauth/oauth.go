package googleoauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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

type Config struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
}

type LoginOptions struct {
	Port int
}

func Login(ctx context.Context, out io.Writer) (credentials.GoogleOAuthCredential, error) {
	return LoginWithOptions(ctx, out, LoginOptions{})
}

func LoginWithOptions(ctx context.Context, out io.Writer, options LoginOptions) (credentials.GoogleOAuthCredential, error) {
	config, err := DefaultConfig()
	if err != nil {
		return credentials.GoogleOAuthCredential{}, err
	}
	return LoginWithConfigOptions(ctx, out, config, options)
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
	}, nil
}

func DefaultScopes() []string {
	return []string{bq.BigqueryScope}
}

func LoginWithConfig(ctx context.Context, out io.Writer, config Config) (credentials.GoogleOAuthCredential, error) {
	return LoginWithConfigOptions(ctx, out, config, LoginOptions{})
}

func LoginWithConfigOptions(ctx context.Context, out io.Writer, config Config, options LoginOptions) (credentials.GoogleOAuthCredential, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	if config.ClientID == "" || config.ClientSecret == "" {
		return credentials.GoogleOAuthCredential{}, errors.New("Google OAuth client ID and client secret are required")
	}
	if len(config.Scopes) == 0 {
		config.Scopes = DefaultScopes()
	}

	state, err := randomState()
	if err != nil {
		return credentials.GoogleOAuthCredential{}, err
	}
	callbackServer, err := startLoopbackServer(state, options.Port)
	if err != nil {
		return credentials.GoogleOAuthCredential{}, fmt.Errorf("start OAuth callback listener: %w", err)
	}
	defer callbackServer.Shutdown()

	oauthConfig := oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Scopes:       config.Scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  callbackServer.RedirectURL(),
	}

	authURL := oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	)
	fmt.Fprintln(out, "Open this URL in a browser on this computer to finish SegmentStream warehouse login:")
	fmt.Fprintln(out, authURL)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "The browser must be able to reach the local callback at %s.\n", oauthConfig.RedirectURL)
	fmt.Fprintln(out, "For headless servers or CI, use segmentstream warehouse auth --service-account-key=<path>.")
	fmt.Fprintln(out, "Waiting for Google OAuth callback. Press Ctrl-C to cancel.")

	code, err := callbackServer.Wait(ctx)
	if err != nil {
		return credentials.GoogleOAuthCredential{}, err
	}
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
