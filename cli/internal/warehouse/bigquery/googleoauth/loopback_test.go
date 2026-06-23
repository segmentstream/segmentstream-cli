package googleoauth

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartLoopbackServerUsesFixedPort(t *testing.T) {
	reservation, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reservation.Addr().(*net.TCPAddr).Port
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}

	server, err := startLoopbackServer("expected-state", port)
	if err != nil {
		t.Fatalf("startLoopbackServer failed: %v", err)
	}
	defer server.Shutdown()

	want := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	if got := server.RedirectURL(); got != want {
		t.Fatalf("RedirectURL = %q, want %q", got, want)
	}
}

func TestStartLoopbackServerRejectsInvalidPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		t.Run(fmt.Sprint(port), func(t *testing.T) {
			server, err := startLoopbackServer("expected-state", port)
			if err == nil {
				server.Shutdown()
				t.Fatal("startLoopbackServer succeeded, want invalid port error")
			}
			if !strings.Contains(err.Error(), "invalid OAuth callback port") {
				t.Fatalf("error = %v, want invalid OAuth callback port", err)
			}
		})
	}
}

func TestCallbackHandlerAcceptsMatchingStateAndCode(t *testing.T) {
	resultCh := make(chan callbackResult, 1)
	handler := newCallbackHandler("expected-state", resultCh)
	request := httptest.NewRequest(http.MethodGet, "/callback?state=expected-state&code=auth-code", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	result := <-resultCh
	if result.err != nil || result.code != "auth-code" {
		t.Fatalf("result = %+v, want auth code", result)
	}
}

func TestCallbackHandlerRejectsStateMismatch(t *testing.T) {
	resultCh := make(chan callbackResult, 1)
	handler := newCallbackHandler("expected-state", resultCh)
	request := httptest.NewRequest(http.MethodGet, "/callback?state=wrong-state&code=auth-code", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil || !strings.Contains(result.err.Error(), "state mismatch") {
		t.Fatalf("result = %+v, want state mismatch error", result)
	}
}

func TestCallbackHandlerRejectsOAuthError(t *testing.T) {
	resultCh := make(chan callbackResult, 1)
	handler := newCallbackHandler("expected-state", resultCh)
	request := httptest.NewRequest(http.MethodGet, "/callback?state=expected-state&error=access_denied&error_description=denied", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil || !strings.Contains(result.err.Error(), "access_denied denied") {
		t.Fatalf("result = %+v, want OAuth error", result)
	}
}

func TestCallbackHandlerRejectsMissingCode(t *testing.T) {
	resultCh := make(chan callbackResult, 1)
	handler := newCallbackHandler("expected-state", resultCh)
	request := httptest.NewRequest(http.MethodGet, "/callback?state=expected-state", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	result := <-resultCh
	if result.err == nil || !strings.Contains(result.err.Error(), "authorization code is missing") {
		t.Fatalf("result = %+v, want missing code error", result)
	}
}
