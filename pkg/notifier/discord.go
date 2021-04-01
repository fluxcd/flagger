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
	"path"
	"strings"
)

// Discord holds the hook URL
type Discord struct {
	URL      string
	ProxyURL string
	Username string
	Channel  string
}

// NewDiscord validates the URL and returns a Discord object
func NewDiscord(hookURL string, proxyURL string, username string, channel string) (*Discord, error) {
	webhook, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Discord hook URL %s", hookURL)
	}

	// use Slack formatting
	// https://birdie0.github.io/discord-webhooks-guide/other/slack_formatting.html
	if !strings.HasSuffix(hookURL, "/slack") {
		webhook.Path = path.Join(webhook.Path, "slack")
		hookURL = webhook.String()
	}

	if username == "" {
		return nil, errors.New("empty Discord username")
	}

	if channel == "" {
		return nil, errors.New("empty Discord channel")
	}

	return &Discord{
		Channel:  channel,
		URL:      hookURL,
		ProxyURL: proxyURL,
		Username: username,
	}, nil
}

// Post Discord message
func (s *Discord) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	payload := SlackPayload{
		Channel:   s.Channel,
		Username:  s.Username,
		IconEmoji: ":rocket:",
	}

	color := "good"
	if severity == "error" {
		color = "danger"
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
