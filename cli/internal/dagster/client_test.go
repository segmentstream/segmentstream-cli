package dagster

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitUntilReadyRetriesWithExponentialBackoff(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		writeGraphQLData(t, w, map[string]any{"version": "1.0.0"})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if err := client.WaitUntilReady(context.Background()); err != nil {
		t.Fatalf("WaitUntilReady failed: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("ready attempts = %d, want 3", attempts)
	}
}

func TestMaterializableAssetsRetriesTransientStartupFailure(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		writeGraphQLData(t, w, map[string]any{
			"assetNodes": []map[string]any{
				{
					"assetKey":         map[string]any{"path": []string{"events"}},
					"isMaterializable": true,
					"isPartitioned":    true,
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	assets, err := client.MaterializableAssets(context.Background())
	if err != nil {
		t.Fatalf("MaterializableAssets failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("asset attempts = %d, want 2", attempts)
	}
	if len(assets) != 1 || assets[0].Key[0] != "events" {
		t.Fatalf("assets = %+v, want events asset", assets)
	}
}

func TestMaterializableAssetsRetriesEOF(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			closeWithoutResponse(t, w, r)
			return
		}
		writeGraphQLData(t, w, map[string]any{
			"assetNodes": []map[string]any{
				{
					"assetKey":         map[string]any{"path": []string{"events"}},
					"isMaterializable": true,
					"isPartitioned":    true,
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	assets, err := client.MaterializableAssets(context.Background())
	if err != nil {
		t.Fatalf("MaterializableAssets failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("asset attempts = %d, want retry after EOF", attempts)
	}
	if len(assets) != 1 || assets[0].Key[0] != "events" {
		t.Fatalf("assets = %+v, want events asset", assets)
	}
}

func TestLaunchBackfillDoesNotRetryEOF(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		closeWithoutResponse(t, w, r)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.LaunchBackfill(context.Background(), []AssetNode{
		{Key: []string{"events"}, IsPartitioned: true},
	}, DateRange{
		StartDate:        "2026-05-19",
		EndInclusiveDate: "2026-06-17",
	}); err == nil {
		t.Fatal("expected EOF error from LaunchBackfill")
	}
	// A dropped connection on a mutation leaves the backfill in an unknown
	// state; retrying could launch a second backfill. The launch must fail
	// closed rather than retry.
	if attempts != 1 {
		t.Fatalf("launch attempts = %d, want no retry on EOF to avoid double-start", attempts)
	}
}

func TestMaterializableAssetsDoesNotRetryGraphQLErrors(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{{"message": "repository failed to load"}},
		}); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.MaterializableAssets(context.Background()); err == nil {
		t.Fatal("expected GraphQL error")
	}
	if attempts != 1 {
		t.Fatalf("asset attempts = %d, want no retry for GraphQL errors", attempts)
	}
}

func TestMaterializableAssetsFiltersAndSorts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGraphQLData(t, w, map[string]any{
			"assetNodes": []map[string]any{
				{
					"assetKey":         map[string]any{"path": []string{"z_asset"}},
					"isMaterializable": true,
					"isPartitioned":    false,
				},
				{
					"assetKey":         map[string]any{"path": []string{"raw_source"}},
					"isMaterializable": false,
					"isPartitioned":    true,
				},
				{
					"assetKey":         map[string]any{"path": []string{"events"}},
					"isMaterializable": true,
					"isPartitioned":    true,
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	assets, err := client.MaterializableAssets(context.Background())
	if err != nil {
		t.Fatalf("MaterializableAssets failed: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("assets = %+v, want 2 materializable assets", assets)
	}
	if got := assets[0]; got.Key[0] != "events" || !got.IsPartitioned {
		t.Fatalf("first asset = %+v, want partitioned events", got)
	}
	if got := assets[1]; got.Key[0] != "z_asset" || got.IsPartitioned {
		t.Fatalf("second asset = %+v, want unpartitioned z_asset", got)
	}
}

func TestLaunchBackfillUsesPartitionsByAssets(t *testing.T) {
	var request graphQLTestRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeGraphQLData(t, w, map[string]any{
			"launchPartitionBackfill": map[string]any{
				"__typename": "LaunchBackfillSuccess",
				"backfillId": "__backfill_id",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	backfillID, err := client.LaunchBackfill(context.Background(), []AssetNode{
		{Key: []string{"events"}, IsPartitioned: true},
		{Key: []string{"dimension_table"}, IsPartitioned: false},
	}, DateRange{
		StartDate:        "2026-05-19",
		EndInclusiveDate: "2026-06-17",
	})
	if err != nil {
		t.Fatalf("LaunchBackfill failed: %v", err)
	}
	if backfillID != "__backfill_id" {
		t.Fatalf("backfill id = %q, want __backfill_id", backfillID)
	}

	params := request.Variables["backfillParams"].(map[string]any)
	partitionsByAssets := params["partitionsByAssets"].([]any)
	if len(partitionsByAssets) != 2 {
		t.Fatalf("partitionsByAssets = %+v, want 2 assets", partitionsByAssets)
	}
	partitioned := partitionsByAssets[0].(map[string]any)
	rangeSelector := partitioned["partitions"].(map[string]any)["range"].(map[string]any)
	if rangeSelector["start"] != "2026-05-19" || rangeSelector["end"] != "2026-06-17" {
		t.Fatalf("partition range = %+v, want inclusive run range", rangeSelector)
	}
	unpartitioned := partitionsByAssets[1].(map[string]any)
	if unpartitioned["partitions"] != nil {
		t.Fatalf("unpartitioned asset partitions = %+v, want null", unpartitioned["partitions"])
	}
}

func TestLaunchBackfillRetriesServiceUnavailable(t *testing.T) {
	withFastGraphQLRetries(t)

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		writeGraphQLData(t, w, map[string]any{
			"launchPartitionBackfill": map[string]any{
				"__typename": "LaunchBackfillSuccess",
				"backfillId": "__backfill_id",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	backfillID, err := client.LaunchBackfill(context.Background(), []AssetNode{
		{Key: []string{"events"}, IsPartitioned: true},
	}, DateRange{
		StartDate:        "2026-05-19",
		EndInclusiveDate: "2026-06-17",
	})
	if err != nil {
		t.Fatalf("LaunchBackfill failed: %v", err)
	}
	if backfillID != "__backfill_id" {
		t.Fatalf("backfill id = %q, want __backfill_id", backfillID)
	}
	if attempts != 2 {
		t.Fatalf("launch attempts = %d, want 2", attempts)
	}
}

func TestWaitForBackfillFailsTerminalStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGraphQLData(t, w, map[string]any{
			"partitionBackfillOrError": map[string]any{
				"__typename": "PartitionBackfill",
				"status":     "COMPLETED_FAILED",
				"error":      map[string]any{"message": "warehouse query failed"},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL)
	err := client.WaitForBackfill(context.Background(), "__backfill_id")
	if err == nil {
		t.Fatal("expected failed backfill status")
	}
	if got, want := err.Error(), "pipeline finished with status COMPLETED_FAILED: warehouse query failed"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

type graphQLTestRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// closeWithoutResponse drains the request and closes the connection without
// writing a response, so the client observes `Post ...: EOF` — the failure
// macOS users hit when Docker Desktop's proxy drops an idle connection.
func closeWithoutResponse(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	_, _ = io.Copy(io.Discard, r.Body)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		t.Fatal("response writer does not support hijacking")
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("hijack connection: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close connection: %v", err)
	}
}

func writeGraphQLData(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func withFastGraphQLRetries(t *testing.T) {
	t.Helper()

	previousReadyTimeout := readyTimeout
	previousGraphQLRetryTimeout := graphQLRetryTimeout
	previousInitialDelay := graphQLRetryInitialDelay
	previousMaxDelay := graphQLRetryMaxDelay

	readyTimeout = time.Second
	graphQLRetryTimeout = time.Second
	graphQLRetryInitialDelay = time.Millisecond
	graphQLRetryMaxDelay = 2 * time.Millisecond

	t.Cleanup(func() {
		readyTimeout = previousReadyTimeout
		graphQLRetryTimeout = previousGraphQLRetryTimeout
		graphQLRetryInitialDelay = previousInitialDelay
		graphQLRetryMaxDelay = previousMaxDelay
	})
}
