package googleoauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const callbackPath = "/callback"

type loopbackServer struct {
	listener net.Listener
	server   *http.Server
	resultCh chan callbackResult
}

type callbackResult struct {
	code string
	err  error
}

func startLoopbackServer(state string, port int) (*loopbackServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid OAuth callback port %d; use 0-65535", port)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}

	resultCh := make(chan callbackResult, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           newCallbackHandler(state, resultCh),
	}
	loopback := &loopbackServer{
		listener: listener,
		server:   server,
		resultCh: resultCh,
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sendCallbackResult(resultCh, callbackResult{err: err})
		}
	}()

	return loopback, nil
}

func (server *loopbackServer) RedirectURL() string {
	return "http://" + server.listener.Addr().String() + callbackPath
}

func (server *loopbackServer) Wait(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-server.resultCh:
		if result.err != nil {
			return "", result.err
		}
		return result.code, nil
	}
}

func (server *loopbackServer) Shutdown() {
	_ = server.server.Shutdown(context.Background())
	_ = server.listener.Close()
}

func newCallbackHandler(state string, resultCh chan<- callbackResult) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callbackPath {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("state"); got != state {
			http.Error(w, "OAuth state mismatch. Return to the terminal and retry.", http.StatusBadRequest)
			sendCallbackResult(resultCh, callbackResult{err: errors.New("OAuth state mismatch")})
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			description := r.URL.Query().Get("error_description")
			http.Error(w, "OAuth authorization failed. Return to the terminal for details.", http.StatusBadRequest)
			sendCallbackResult(resultCh, callbackResult{err: fmt.Errorf("OAuth authorization failed: %s %s", oauthErr, description)})
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "OAuth authorization code is missing. Return to the terminal and retry.", http.StatusBadRequest)
			sendCallbackResult(resultCh, callbackResult{err: errors.New("OAuth authorization code is missing")})
			return
		}
		fmt.Fprintln(w, "SegmentStream warehouse login succeeded. You can return to the terminal.")
		sendCallbackResult(resultCh, callbackResult{code: code})
	})
}

func sendCallbackResult(resultCh chan<- callbackResult, result callbackResult) {
	select {
	case resultCh <- result:
	default:
	}
}
