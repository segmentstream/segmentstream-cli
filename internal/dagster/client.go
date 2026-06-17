package dagster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
)

const graphQLPath = "/graphql"

var (
	readyTimeout      = 2 * time.Minute
	readyPollInterval = time.Second
	backfillInterval  = 5 * time.Second
)

type Client interface {
	WaitUntilReady(context.Context) error
	MaterializableAssets(context.Context) ([]AssetNode, error)
	LaunchBackfill(context.Context, []AssetNode, DateRange) (string, error)
	WaitForBackfill(context.Context, string) error
}

type AssetNode struct {
	Key           []string
	IsPartitioned bool
}

type DateRange struct {
	StartDate        string
	EndInclusiveDate string
}

type graphQLClient struct {
	client graphql.Client
}

func NewClient(baseURL string) Client {
	return &graphQLClient{
		client: graphql.NewClient(
			strings.TrimRight(baseURL, "/")+graphQLPath,
			&http.Client{Timeout: 10 * time.Second},
		),
	}
}

func (c *graphQLClient) WaitUntilReady(ctx context.Context) error {
	deadline := time.NewTimer(readyTimeout)
	defer deadline.Stop()

	ticker := time.NewTicker(readyPollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		if _, err := SegmentStreamReady(ctx, c.client); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if lastErr != nil {
				return fmt.Errorf("SegmentStream did not become ready: %w", lastErr)
			}
			return errors.New("SegmentStream did not become ready")
		case <-ticker.C:
		}
	}
}

func (c *graphQLClient) MaterializableAssets(ctx context.Context) ([]AssetNode, error) {
	response, err := SegmentStreamAssets(ctx, c.client)
	if err != nil {
		return nil, err
	}

	assets := make([]AssetNode, 0, len(response.AssetNodes))
	for _, node := range response.AssetNodes {
		if !node.IsMaterializable {
			continue
		}
		assets = append(assets, AssetNode{
			Key:           append([]string(nil), node.AssetKey.Path...),
			IsPartitioned: node.IsPartitioned,
		})
	}
	sort.Slice(assets, func(i, j int) bool {
		return strings.Join(assets[i].Key, "\x00") < strings.Join(assets[j].Key, "\x00")
	})
	return assets, nil
}

func (c *graphQLClient) LaunchBackfill(ctx context.Context, assets []AssetNode, runRange DateRange) (string, error) {
	title := fmt.Sprintf("SegmentStream run %s through %s", runRange.StartDate, runRange.EndInclusiveDate)
	description := "Launched by segmentstream run."
	response, err := SegmentStreamLaunchBackfill(ctx, c.client, LaunchBackfillParams{
		PartitionsByAssets: partitionsByAssetsInput(assets, runRange),
		Tags: []ExecutionTag{
			{Key: "segmentstream/source", Value: "cli"},
		},
		Title:       &title,
		Description: &description,
	})
	if err != nil {
		return "", err
	}

	switch result := response.LaunchPartitionBackfill.(type) {
	case *SegmentStreamLaunchBackfillLaunchPartitionBackfillLaunchBackfillSuccess:
		if result.BackfillId == "" {
			return "", errors.New("backfill launch returned an empty id")
		}
		return result.BackfillId, nil
	case nil:
		return "", errors.New("backfill launch returned no result")
	default:
		return "", graphQLResultError(result, "backfill launch")
	}
}

func partitionsByAssetsInput(assets []AssetNode, runRange DateRange) []*PartitionsByAssetSelector {
	inputs := make([]*PartitionsByAssetSelector, 0, len(assets))
	for _, asset := range assets {
		input := &PartitionsByAssetSelector{
			AssetKey: AssetKeyInput{Path: asset.Key},
		}
		if asset.IsPartitioned {
			input.Partitions = &PartitionsSelector{
				Range: &PartitionRangeSelector{
					Start: runRange.StartDate,
					End:   runRange.EndInclusiveDate,
				},
			}
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (c *graphQLClient) WaitForBackfill(ctx context.Context, backfillID string) error {
	ticker := time.NewTicker(backfillInterval)
	defer ticker.Stop()

	for {
		status, detail, err := c.backfillStatus(ctx, backfillID)
		if err != nil {
			return err
		}

		switch status {
		case "COMPLETED", "COMPLETED_SUCCESS":
			return nil
		case "FAILED", "CANCELED", "CANCELING", "FAILING", "COMPLETED_FAILED":
			if detail != "" {
				return fmt.Errorf("pipeline finished with status %s: %s", status, detail)
			}
			return fmt.Errorf("pipeline finished with status %s", status)
		case "REQUESTED":
		default:
			if status != "" {
				return fmt.Errorf("pipeline returned unknown status %s", status)
			}
			return errors.New("pipeline returned empty status")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *graphQLClient) backfillStatus(ctx context.Context, backfillID string) (string, string, error) {
	response, err := SegmentStreamBackfillStatus(ctx, c.client, backfillID)
	if err != nil {
		return "", "", err
	}

	switch result := response.PartitionBackfillOrError.(type) {
	case *SegmentStreamBackfillStatusPartitionBackfillOrErrorPartitionBackfill:
		detail := ""
		if result.Error != nil {
			detail = result.Error.Message
		}
		return result.Status, detail, nil
	case nil:
		return "", "", errors.New("backfill status returned no result")
	default:
		return "", "", graphQLResultError(result, "backfill status")
	}
}

type graphQLMessage interface {
	GetMessage() string
}

type graphQLTypename interface {
	GetTypename() *string
}

func graphQLResultError(result any, operation string) error {
	if withMessage, ok := result.(graphQLMessage); ok && withMessage.GetMessage() != "" {
		return errors.New(withMessage.GetMessage())
	}
	if withType, ok := result.(graphQLTypename); ok && withType.GetTypename() != nil {
		return fmt.Errorf("%s returned %s", operation, *withType.GetTypename())
	}
	return fmt.Errorf("%s returned %T", operation, result)
}
