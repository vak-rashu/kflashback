// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

const summarizerSystemPrompt = `You are an expert Kubernetes administrator analyzing resource changes.
Given a JSON merge patch (RFC 7386) showing what changed in a Kubernetes resource, provide a clear, 
concise, human-readable summary of the changes.

Rules:
- Be specific: mention exact values (replica counts, image tags, port numbers).
- Be brief: 1-3 sentences maximum.
- Use plain language, not JSON paths.
- If the patch is empty or trivial, say "No significant changes."
- Group related changes together.

Examples:
- "Scaled replicas from 3 to 5 and updated container image from nginx:1.24 to nginx:1.25."
- "Added resource limits (CPU: 500m, memory: 256Mi) to the main container."
- "Changed service type from ClusterIP to LoadBalancer and added port 443."
`

// Summarizer generates human-readable summaries of Kubernetes resource changes.
type Summarizer struct {
	provider Provider
}

// NewSummarizer creates a new change summarizer.
func NewSummarizer(provider Provider) *Summarizer {
	return &Summarizer{provider: provider}
}

// SummarizeChange generates a human-readable summary of a change.
func (s *Summarizer) SummarizeChange(ctx context.Context, kind, name, namespace string, eventType string, patch json.RawMessage) (string, error) {
	prompt := fmt.Sprintf(`Kubernetes %s "%s" in namespace "%s" was %s.

Here is the JSON merge patch showing the changes:
%s

Summarize what changed in plain English.`, kind, name, namespace, eventTypeLabel(eventType), string(patch))

	return s.provider.Complete(ctx, CompletionRequest{
		SystemPrompt: summarizerSystemPrompt,
		UserPrompt:   prompt,
	})
}

// SummarizeDiff generates a human-readable summary comparing two full resource snapshots.
func (s *Summarizer) SummarizeDiff(ctx context.Context, kind, name, namespace string, fromSnapshot, toSnapshot json.RawMessage) (string, error) {
	prompt := fmt.Sprintf(`Compare these two versions of Kubernetes %s "%s" in namespace "%s".

BEFORE:
%s

AFTER:
%s

Summarize the key differences in plain English.`, kind, name, namespace, truncateJSON(fromSnapshot, 4000), truncateJSON(toSnapshot, 4000))

	return s.provider.Complete(ctx, CompletionRequest{
		SystemPrompt: summarizerSystemPrompt,
		UserPrompt:   prompt,
	})
}

func eventTypeLabel(eventType string) string {
	switch eventType {
	case "CREATED":
		return "created"
	case "UPDATED":
		return "updated"
	case "DELETED":
		return "deleted"
	default:
		return "modified"
	}
}

func truncateJSON(data json.RawMessage, maxLen int) string {
	s := string(data)
	if len(s) > maxLen {
		return s[:maxLen] + "... (truncated)"
	}
	return s
}
