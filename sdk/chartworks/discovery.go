package chartworks

import (
	"context"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

// TopicListRequest selects a bounded authorized publication page.
type TopicListRequest = topics.ListRequest

// TopicSummary is retained public metadata, not current warehouse health.
type TopicSummary = topics.Summary

// DatasetListRequest selects a bounded page in one exact registered context.
type DatasetListRequest = sources.DatasetListRequest

// DatasetDescribeRequest selects one registered dataset.
type DatasetDescribeRequest = sources.DatasetDescribeRequest

// Dataset describes registered metadata, not result rows.
type Dataset = sources.Dataset

// ListTopics reads authorized active publications without model/source calls.
func (c *Client) ListTopics(ctx context.Context, in TopicListRequest) (out []TopicSummary, err error) {
	err = c.call(ctx, "POST", "/v1/topics/list", "", in, &out)
	return
}

// ListDatasets reads registered metadata without a warehouse call.
func (c *Client) ListDatasets(ctx context.Context, in DatasetListRequest) (out []Dataset, err error) {
	err = c.call(ctx, "POST", "/v1/datasets/list", "", in, &out)
	return
}

// DescribeDataset reads one registered dataset in its exact context.
func (c *Client) DescribeDataset(ctx context.Context, in DatasetDescribeRequest) (out Dataset, err error) {
	err = c.call(ctx, "POST", "/v1/datasets/describe", "", in, &out)
	return
}
