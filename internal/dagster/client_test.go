package dagster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func writeGraphQLData(t *testing.T, w http.ResponseWriter, data map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
