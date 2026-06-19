package googleoauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
