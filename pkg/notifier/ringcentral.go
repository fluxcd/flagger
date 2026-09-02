/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notifier

import (
	"fmt"
	"net/url"
	"strings"
)

// RingCentral holds the incoming webhook URL
type RingCentral struct {
	URL      string
	ProxyURL string
}

// RingCentralPayload holds the message card data
type RingCentralPayload struct {
	Activity    string                  `json:"activity"`
	Attachments []RingCentralAttachment `json:"attachments"`
}

// RingCentralAttachment holds the canary analysis result
type RingCentralAttachment struct {
	Schema  string                   `json:"$schema"`
	Type    string                   `json:"type"`
	Version string                   `json:"version"`
	Body    []map[string]interface{} `json:"body"`
}

// NewRingCentral validates the RingCentral URL and returns a RingCentral object
func NewRingCentral(hookURL string, proxyURL string) (*RingCentral, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid RingCentral webhook URL %s", hookURL)
	}

	return &RingCentral{
		URL:      hookURL,
		ProxyURL: proxyURL,
	}, nil
}

// Post RingCentral message
func (s *RingCentral) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	payload := RingCentralPayload{
		Activity: fmt.Sprintf("%s.%s", workload, namespace),
	}

	var statusEmoji string
	switch strings.ToLower(severity) {
	case "info":
		statusEmoji = "\u2705" // Check Mark
	case "error":
		statusEmoji = "\u274C" // Cross Mark
	default:
		statusEmoji = "" // No emoji for other severities
	}

	body := []map[string]interface{}{
		{
			"type":   "TextBlock",
			"text":   fmt.Sprintf("%s %s", statusEmoji, message),
			"wrap":   true,
			"weight": "bolder",
			"size":   "medium",
		},
	}

	for _, f := range fields {
		body = append(body, map[string]interface{}{
			"type": "FactSet",
			"facts": []map[string]string{
				{
					"title": f.Name,
					"value": f.Value,
				},
			},
		})
	}

	attachment := RingCentralAttachment{
		Schema:  "http://adaptivecards.io/schemas/adaptive-card.json",
		Type:    "AdaptiveCard",
		Version: "1.0",
		Body:    body,
	}

	payload.Attachments = []RingCentralAttachment{attachment}

	err := postMessage(s.URL, "", s.ProxyURL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}

	return nil
}
