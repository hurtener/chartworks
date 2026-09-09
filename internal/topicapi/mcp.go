package topicapi

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// PublishedTopicRequest names an active reviewed topic, never a private draft.
type PublishedTopicRequest struct {
	Topic string `json:"topic"`
}

// MCPBindings binds only the installed service, never an unavailable placeholder.
func MCPBindings(service *topics.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := Registry()
	if err != nil {
		return nil, err
	}
	var bindings []mcpserver.Binding
	mapper := func(err error) mcpserver.Fault { _, code := classify(err); return mcpserver.Fault{Code: code} }
	b0, err := mcpserver.Bind(registry, "listTopics", "list_topics", "discovery", "List active published topics only when every signed topic, source, dataset and context dependency is reachable. Use an empty after for the first page and a limit from 1 to 100. No private drafts or inference.", service.List, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b0)
	b1, err := mcpserver.Bind(registry, "getPublishedTopic", "describe_topic", "discovery", "Read the current immutable publication of an authorized topic. Private drafts and live source probes are excluded; publication alone does not authorize a later query.", func(ctx context.Context, e identity.Envelope, in PublishedTopicRequest) (topics.Published, error) {
		return service.Read(ctx, e, in.Topic, "")
	}, mapper)
	if err != nil {
		return nil, err
	}
	b1, err = mcpserver.WithResource(b1, "chartworks://topics/{topic}")
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b1)
	return bindings, nil
}
