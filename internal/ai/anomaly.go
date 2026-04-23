// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const anomalySystemPrompt = `You are a Kubernetes security and operations analyst.
Analyze a series of recent changes to Kubernetes resources and identify potential anomalies or concerns.

Flag issues such as:
- Unexpected resource deletions (especially in production namespaces)
- Significant scale-downs that could cause outages
- Container image changes to unrecognized or "latest" tags
- Removal of resource limits or security contexts
- Changes outside normal business hours (if timestamps suggest it)
- Configuration drift from best practices
- Privilege escalation (adding hostNetwork, privileged containers, etc.)

Respond in JSON format with this structure:
{
  "anomalies": [
    {
      "severity": "high|medium|low",
      "title": "Brief title",
      "description": "Explanation of why this is concerning",
      "resource": "kind/namespace/name",
      "revision": <revision_number>
    }
  ],
  "summary": "One sentence overall assessment"
}

If nothing is anomalous, return {"anomalies": [], "summary": "No anomalies detected."}.
`

// Anomaly represents a detected anomaly in resource changes.
type Anomaly struct {
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Resource    string `json:"resource"`
	Revision    int64  `json:"revision"`
}

// AnomalyReport is the result of anomaly detection.
type AnomalyReport struct {
	Anomalies []Anomaly `json:"anomalies"`
	Summary   string    `json:"summary"`
}

// AnomalyDetector analyzes resource changes for anomalies.
type AnomalyDetector struct {
	provider Provider
}

// NewAnomalyDetector creates a new anomaly detector.
func NewAnomalyDetector(provider Provider) *AnomalyDetector {
	return &AnomalyDetector{provider: provider}
}

// ChangeEvent represents a single change for anomaly analysis.
type ChangeEvent struct {
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Revision  int64           `json:"revision"`
	EventType string          `json:"eventType"`
	Timestamp time.Time       `json:"timestamp"`
	Patch     json.RawMessage `json:"patch,omitempty"`
}

// AnalyzeChanges analyzes a set of recent changes for anomalies.
func (d *AnomalyDetector) AnalyzeChanges(ctx context.Context, changes []ChangeEvent) (*AnomalyReport, error) {
	if len(changes) == 0 {
		return &AnomalyReport{Summary: "No changes to analyze."}, nil
	}

	// Build compact text summary instead of raw JSON
	var lines string
	for _, c := range changes {
		ns := c.Namespace
		if ns == "" {
			ns = "cluster"
		}
		lines += fmt.Sprintf("- %s %s/%s (%s) rev %d at %s\n",
			c.EventType, ns, c.Name, c.Kind, c.Revision,
			c.Timestamp.Format("15:04 Jan 02"))
	}

	prompt := fmt.Sprintf(`Analyze these %d recent Kubernetes resource changes for anomalies:

%s
Current time: %s`, len(changes), lines, time.Now().UTC().Format(time.RFC3339))

	response, err := d.provider.Complete(ctx, CompletionRequest{
		SystemPrompt: anomalySystemPrompt,
		UserPrompt:   prompt,
	})
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	var report AnomalyReport
	if err := json.Unmarshal([]byte(response), &report); err != nil {
		// If the response isn't valid JSON, wrap it as a summary
		return &AnomalyReport{
			Summary: response,
		}, nil
	}

	return &report, nil
}
