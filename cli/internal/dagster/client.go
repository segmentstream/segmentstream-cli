package dagster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Khan/genqlient/graphql"
)

const graphQLPath = "/graphql"

var (
	readyTimeout             = 2 * time.Minute
	graphQLRetryTimeout      = 2 * time.Minute
	graphQLRetryInitialDelay = 500 * time.Millisecond
	graphQLRetryMaxDelay     = 5 * time.Second
	backfillInterval         = 5 * time.Second
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
	// Disable HTTP keep-alives so each GraphQL request uses a fresh connection.
	// On macOS, Docker Desktop proxies host->container traffic through a
	// userspace network stack that reaps idle connections aggressively; reusing
	// a pooled connection it has already closed surfaces as
	// `Post ".../graphql": EOF`. LaunchBackfill is a non-idempotent mutation
	// that must not be retried, so we prevent the stale-connection race instead
	// of retrying through it.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true

	return &graphQLClient{
		client: graphql.NewClient(
			strings.TrimRight(baseURL, "/")+graphQLPath,
			&http.Client{
				Timeout:   10 * time.Second,
				Transport: transport,
			},
		),
	}
}

func (c *graphQLClient) WaitUntilReady(ctx context.Context) error {
	err := retryWithExponentialBackoff(ctx, readyTimeout, retryAnyError, func(ctx context.Context) error {
		_, err := SegmentStreamReady(ctx, c.client)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("SegmentStream did not become ready: %w", err)
	}
	return nil
}

func (c *graphQLClient) MaterializableAssets(ctx context.Context) ([]AssetNode, error) {
	var response *SegmentStreamAssetsResponse
	if err := retryTransientGraphQLOperation(ctx, func(ctx context.Context) error {
		var err error
		response, err = SegmentStreamAssets(ctx, c.client)
		return err
	}); err != nil {
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
	var response *SegmentStreamLaunchBackfillResponse
	if err := retryUnavailableGraphQLOperation(ctx, func(ctx context.Context) error {
		var err error
		response, err = SegmentStreamLaunchBackfill(ctx, c.client, LaunchBackfillParams{
			PartitionsByAssets: partitionsByAssetsInput(assets, runRange),
			Tags: []ExecutionTag{
				{Key: "segmentstream/source", Value: "cli"},
			},
			Title:       &title,
			Description: &description,
		})
		return err
	}); err != nil {
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
	var response *SegmentStreamBackfillStatusResponse
	if err := retryTransientGraphQLOperation(ctx, func(ctx context.Context) error {
		var err error
		response, err = SegmentStreamBackfillStatus(ctx, c.client, backfillID)
		return err
	}); err != nil {
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

func retryTransientGraphQLOperation(ctx context.Context, operation func(context.Context) error) error {
	return retryWithExponentialBackoff(ctx, graphQLRetryTimeout, isTransientGraphQLError, operation)
}

func retryUnavailableGraphQLOperation(ctx context.Context, operation func(context.Context) error) error {
	return retryWithExponentialBackoff(ctx, graphQLRetryTimeout, isUnavailableGraphQLError, operation)
}

func retryAnyError(error) bool {
	return true
}

func retryWithExponentialBackoff(ctx context.Context, timeout time.Duration, shouldRetry func(error) bool, operation func(context.Context) error) error {
	retryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	delay := graphQLRetryInitialDelay
	if delay <= 0 {
		delay = time.Millisecond
	}
	if graphQLRetryMaxDelay > 0 && delay > graphQLRetryMaxDelay {
		delay = graphQLRetryMaxDelay
	}

	var lastErr error
	for {
		err := operation(retryCtx)
		if err == nil {
			return nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return ctx.Err()
		}
		if retryCtx.Err() != nil {
			return graphQLRetryTimeoutError(timeout, lastErr)
		}
		if !shouldRetry(err) {
			return err
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-retryCtx.Done():
			timer.Stop()
			return graphQLRetryTimeoutError(timeout, lastErr)
		case <-timer.C:
		}

		delay = nextRetryDelay(delay)
	}
}

func nextRetryDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return time.Millisecond
	}
	next := delay * 2
	if next < delay {
		if graphQLRetryMaxDelay > 0 {
			return graphQLRetryMaxDelay
		}
		return delay
	}
	if graphQLRetryMaxDelay > 0 && next > graphQLRetryMaxDelay {
		return graphQLRetryMaxDelay
	}
	return next
}

func graphQLRetryTimeoutError(timeout time.Duration, lastErr error) error {
	if lastErr != nil {
		return fmt.Errorf("GraphQL request did not succeed within %s: %w", timeout, lastErr)
	}
	return fmt.Errorf("GraphQL request did not succeed within %s", timeout)
}

func isTransientGraphQLError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if isUnavailableGraphQLError(err) {
		return true
	}
	// A dropped connection (the server or Docker Desktop's proxy closing a
	// socket) surfaces as io.EOF. Retrying is safe for the idempotent query
	// operations that use this classifier. The LaunchBackfill mutation uses
	// isUnavailableGraphQLError and is deliberately excluded so an ambiguous
	// EOF never launches a second backfill.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection reset by peer",
		"connection aborted",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isUnavailableGraphQLError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"connection refused",
		"actively refused",
		"connectex",
		"no such host",
		"temporary failure",
		"server closed idle connection",
		"returned error 502",
		"returned error 503",
		"returned error 504",
		"bad gateway",
		"service unavailable",
		"gateway timeout",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
