// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kflashback/kflashback/internal/storage"
)

const querySystemPrompt = `You are a helpful Kubernetes cluster historian.
Answer the user's question based on the provided cluster summary.
Be specific — reference resource names, namespaces, and counts.
Keep answers concise. Use bullet points for lists.
If the data doesn't answer the question, say so.
`

// QueryEngine handles natural language queries about cluster changes.
type QueryEngine struct {
	provider    Provider
	store       storage.Store
	contextMode string // "compact" or "full"
}

// NewQueryEngine creates a new natural language query engine.
// contextMode: "compact" (fast, for local models) or "full" (detailed JSON, for cloud models).
func NewQueryEngine(provider Provider, store storage.Store, contextMode string) *QueryEngine {
	if contextMode == "" {
		contextMode = "compact"
	}
	return &QueryEngine{provider: provider, store: store, contextMode: contextMode}
}

// QueryResult is the response to a natural language query.
type QueryResult struct {
	Answer  string        `json:"answer"`
	Sources []QuerySource `json:"sources,omitempty"`
}

// QuerySource references a specific resource/revision that contributed to the answer.
type QuerySource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Revision  int64  `json:"revision,omitempty"`
}

// Ask answers a natural language question about cluster changes.
func (q *QueryEngine) Ask(ctx context.Context, question string) (*QueryResult, error) {
	// Gather context data from the store
	contextData, err := q.gatherContext(ctx, question)
	if err != nil {
		return nil, fmt.Errorf("gathering context: %w", err)
	}

	prompt := fmt.Sprintf(`User question: %s

Here is the current cluster data:

%s

Answer the user's question based on this data.`, question, contextData)

	answer, err := q.provider.Complete(ctx, CompletionRequest{
		SystemPrompt: querySystemPrompt,
		UserPrompt:   prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("AI query failed: %w", err)
	}

	return &QueryResult{Answer: answer}, nil
}

// gatherContext builds context from the store based on the configured context mode.
func (q *QueryEngine) gatherContext(ctx context.Context, _ string) (string, error) {
	if q.contextMode == "full" {
		return q.gatherFullContext(ctx)
	}
	return q.gatherCompactContext(ctx)
}

// gatherCompactContext builds a short text summary (fast, for local models).
func (q *QueryEngine) gatherCompactContext(ctx context.Context) (string, error) {
	var b []byte
	buf := func(format string, args ...interface{}) {
		b = append(b, []byte(fmt.Sprintf(format+"\n", args...))...)
	}

	// Stats
	stats, err := q.store.GetStats(ctx)
	if err == nil {
		buf("Cluster: %d tracked resources, %d total revisions", stats.TotalResources, stats.TotalRevisions)
	}

	// Kind breakdown
	kindStats, err := q.store.GetKindStats(ctx)
	if err == nil && len(kindStats) > 0 {
		buf("\nResource types:")
		for _, ks := range kindStats {
			buf("- %s (%s): %d resources, %d revisions", ks.Kind, ks.APIVersion, ks.Resources, ks.Revisions)
		}
	}

	// Tracked resources (compact list with namespaces)
	resources, _, err := q.store.ListTrackedResources(ctx, storage.ResourceListQuery{Limit: 30})
	if err == nil && len(resources) > 0 {
		namespaces := map[string]int{}
		for _, r := range resources {
			ns := r.Namespace
			if ns == "" {
				ns = "cluster-scoped"
			}
			namespaces[ns]++
		}
		buf("\nNamespaces:")
		for ns, count := range namespaces {
			buf("- %s: %d resources", ns, count)
		}
	}

	// Recent changes (compact: one line per change)
	since := time.Now().Add(-24 * time.Hour)
	revisions, _, err := q.store.GetHistory(ctx, storage.ResourceHistoryQuery{
		Since: &since,
		Limit: 20,
	})
	if err == nil && len(revisions) > 0 {
		buf("\nRecent changes (last 24h):")
		for _, rev := range revisions {
			ns := rev.Namespace
			if ns == "" {
				ns = "cluster"
			}
			buf("- %s %s/%s rev %d %s at %s",
				rev.EventType, ns, rev.Name, rev.Revision, rev.Kind,
				rev.Timestamp.Format("15:04 Jan 02"))
		}
	} else {
		buf("\nNo changes in the last 24 hours.")
	}

	return string(b), nil
}

// gatherFullContext builds detailed JSON context (slower, for cloud models like GPT-4, Claude).
func (q *QueryEngine) gatherFullContext(ctx context.Context) (string, error) {
	var sections []string

	stats, err := q.store.GetStats(ctx)
	if err == nil {
		j, _ := json.Marshal(stats)
		sections = append(sections, fmt.Sprintf("## Cluster Stats\n%s", string(j)))
	}

	kindStats, err := q.store.GetKindStats(ctx)
	if err == nil {
		j, _ := json.Marshal(kindStats)
		sections = append(sections, fmt.Sprintf("## Resource Types\n%s", string(j)))
	}

	resources, _, err := q.store.ListTrackedResources(ctx, storage.ResourceListQuery{Limit: 50})
	if err == nil {
		j, _ := json.Marshal(resources)
		sections = append(sections, fmt.Sprintf("## Tracked Resources\n%s", string(j)))
	}

	since := time.Now().Add(-24 * time.Hour)
	revisions, _, err := q.store.GetHistory(ctx, storage.ResourceHistoryQuery{
		Since: &since,
		Limit: 100,
	})
	if err == nil {
		summaries := make([]map[string]interface{}, 0, len(revisions))
		for _, rev := range revisions {
			summaries = append(summaries, map[string]interface{}{
				"kind":      rev.Kind,
				"name":      rev.Name,
				"namespace": rev.Namespace,
				"revision":  rev.Revision,
				"eventType": rev.EventType,
				"timestamp": rev.Timestamp.Format(time.RFC3339),
				"changed":   rev.ChangedFields,
			})
		}
		j, _ := json.Marshal(summaries)
		sections = append(sections, fmt.Sprintf("## Recent Changes (last 24h)\n%s", string(j)))
	}

	result := ""
	for _, s := range sections {
		result += s + "\n\n"
	}
	return result, nil
}
