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
	"errors"
	"fmt"
	"net/url"
)

// Rocket holds the hook URL
type Rocket struct {
	URL      string
	ProxyURL string
	Username string
	Channel  string
}

// NewRocket validates the Rocket URL and returns a Rocket object
func NewRocket(hookURL string, proxyUrl string, username string, channel string) (*Rocket, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Rocket hook URL %s", hookURL)
	}

	if username == "" {
		return nil, errors.New("empty Rocket username")
	}

	if channel == "" {
		return nil, errors.New("empty Rocket channel")
	}

	return &Rocket{
		Channel:  channel,
		URL:      hookURL,
		ProxyURL: proxyUrl,
		Username: username,
	}, nil
}

// Post Rocket message
func (s *Rocket) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	payload := SlackPayload{
		Channel:   s.Channel,
		Username:  s.Username,
		IconEmoji: ":rocket:",
	}

	color := "#0076D7"
	if severity == "error" {
		color = "#FF0000"
	}

	sfields := make([]SlackField, 0, len(fields))
	for _, f := range fields {
		sfields = append(sfields, SlackField{f.Name, f.Value, false})
	}

	a := SlackAttachment{
		Color:      color,
		AuthorName: fmt.Sprintf("%s.%s", workload, namespace),
		Text:       message,
		MrkdwnIn:   []string{"text"},
		Fields:     sfields,
	}

	payload.Attachments = []SlackAttachment{a}

	err := postMessage(s.URL, s.ProxyURL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}
	return nil
}
