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

// Slack holds the hook URL
type Slack struct {
	URL      string
	ProxyURL string
	Username string
	Channel  string
}

// SlackPayload holds the channel and attachments
type SlackPayload struct {
	Channel     string            `json:"channel"`
	Username    string            `json:"username"`
	IconUrl     string            `json:"icon_url"`
	IconEmoji   string            `json:"icon_emoji"`
	Text        string            `json:"text,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackAttachment holds the markdown message body
type SlackAttachment struct {
	Color      string       `json:"color"`
	AuthorName string       `json:"author_name"`
	Text       string       `json:"text"`
	MrkdwnIn   []string     `json:"mrkdwn_in"`
	Fields     []SlackField `json:"fields"`
}

type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// NewSlack validates the Slack URL and returns a Slack object
func NewSlack(hookURL string, proxyURL string, username string, channel string) (*Slack, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Slack hook URL %s", hookURL)
	}

	if username == "" {
		return nil, errors.New("empty Slack username")
	}

	if channel == "" {
		return nil, errors.New("empty Slack channel")
	}

	return &Slack{
		Channel:  channel,
		URL:      hookURL,
		ProxyURL: proxyURL,
		Username: username,
	}, nil
}

// Post Slack message
func (s *Slack) Post(workload string, namespace string, message string, fields []Field, severity string) error {
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
