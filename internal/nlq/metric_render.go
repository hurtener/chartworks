package nlq

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// A definition identity is scoped to an exact topic version, not a coincidentally
// equal entity label in another topic. Unscoped legacy metrics cannot share it.
type metricDefinitionIdentity struct {
	Topic   string `json:"topic,omitempty"`
	Version string `json:"version,omitempty"`
	Root    string `json:"root,omitempty"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
}

func metricDefinitionScope(input ContextInput, metric PinnedMetric) (string, string) {
	topics := input.Topics
	if len(topics) == 0 && input.Topic != "" {
		topics = []TopicRevision{{Topic: input.Topic, Version: input.TopicVersion}}
	}
	var found *TopicRevision
	for i, topic := range topics {
		matches := false
		for _, dependency := range metric.Dependencies {
			if dependency.Kind != "measure" && dependency.Kind != "kpi" {
				continue
			}
			matches = matches || metric.ID == topic.Topic+":"+dependency.ID || len(topics) == 1 && metric.ID == dependency.ID
		}
		if matches {
			if found != nil {
				return "", ""
			}
			copy := topics[i]
			found = &copy
		}
	}
	if found == nil || found.Topic == "" || found.Version == "" {
		return "", ""
	}
	return found.Topic, found.Version
}

// metricRendering preserves every root-to-dependency edge and every exact body.
// Only byte-identical definitions at the same typed versioned identity share a
// rendered body. Conflicts fail closed before a context seal can be issued.
func metricRendering(input ContextInput) (string, error) {
	definitions := map[metricDefinitionIdentity]string{}
	var order []metricDefinitionIdentity
	edges := make([][]metricDefinitionIdentity, len(input.Metrics))
	shared := false
	for i, metric := range input.Metrics {
		topic, version := metricDefinitionScope(input, metric)
		for _, dependency := range metric.Dependencies {
			id := metricDefinitionIdentity{Topic: topic, Version: version, Kind: dependency.Kind, ID: dependency.ID}
			if topic == "" {
				id.Root = metric.ID
			}
			if prior, ok := definitions[id]; ok {
				if prior != dependency.Text {
					return "", &ValidationError{Code: CodeInvalidValue, Path: "metrics.dependencies.conflict"}
				}
				shared = true
			} else {
				definitions[id] = dependency.Text
				order = append(order, id)
			}
			edges[i] = append(edges[i], id)
		}
	}
	// Preserve the historical rendering whenever there is no repeated body.
	if !shared {
		return renderMetrics(input.Metrics), nil
	}
	keys := map[metricDefinitionIdentity]string{}
	for i, id := range order {
		keys[id] = "d" + strconv.Itoa(i)
	}
	var out strings.Builder
	out.WriteString("metric_definitions:shared-versioned-v1; each root requires every referenced exact topic/version/kind/ID definition below\n")
	for i, metric := range input.Metrics {
		out.WriteString(renderItem(LaneMetrics, metric.ID, metric.Text))
		refs := make([]string, 0, len(edges[i]))
		for _, id := range edges[i] {
			refs = append(refs, keys[id])
		}
		raw, _ := json.Marshal(refs)
		fmt.Fprintf(&out, "metric_dependency_refs[%s]:%s\n", metric.ID, raw)
	}
	type namespace struct {
		Topic   string `json:"topic,omitempty"`
		Version string `json:"version,omitempty"`
		Root    string `json:"unscoped_root,omitempty"`
	}
	namespaces := map[namespace]string{}
	for _, id := range order {
		ns := namespace{id.Topic, id.Version, id.Root}
		if _, ok := namespaces[ns]; !ok {
			name := "n" + strconv.Itoa(len(namespaces))
			namespaces[ns] = name
			raw, _ := json.Marshal(ns)
			fmt.Fprintf(&out, "metric_namespace[%s]:%s\n", name, raw)
		}
	}
	out.WriteString("definition_identity:[namespace,kind,entity_id]\n")
	for _, id := range order {
		ns := namespace{id.Topic, id.Version, id.Root}
		raw, _ := json.Marshal([]string{namespaces[ns], id.Kind, id.ID})
		fmt.Fprintf(&out, "metric_dependency_definition[%s] identity:%s value:%s\n", keys[id], raw, definitions[id])
	}
	return out.String(), nil
}

func renderScopedMetrics(input ContextInput) string {
	value, err := metricRendering(input)
	if err != nil {
		return ""
	} // cloneAndValidateInput rejects this before rendering.
	return value
}
