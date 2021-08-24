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
)

// Google Chat holds the incoming webhook URL
type GChat struct {
	URL      string
	ProxyURL string
}

// GChatPayload holds the message
type GChatPayload struct {
	Cards []GChatCards `json:"cards"`
}

// Start - GChatCards holds the canary analysis result
type GChatCards struct {
	Header   GChatHeader      `json:"header"`
	Sections []*GChatSections `json:"sections"`
}

type GChatHeader struct {
	Title    string `json:"title"`
	SubTitle string `json:"subtitle"`
	ImageUrl string `json:"imageUrl"`
}

type GChatSections struct {
	Widgets []GChatWidgets `json:"widgets"`
}

type GChatWidgets struct {
	TextParagraph GChatText `json:"textParagraph"`
}

type GChatField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GChatText struct {
	Text string `json:"text"`
}

// NewGChat validates the GChat URL and returns a GChat object
func NewGChat(hookURL string, proxyURL string) (*GChat, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Google Chat webhook URL %s", hookURL)
	}

	return &GChat{
		URL:      hookURL,
		ProxyURL: proxyURL,
	}, nil
}

// Post Google Chat message
func (s *GChat) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	facts := make([]*GChatSections, 0, len(fields))
	facts = append(facts, &GChatSections{
		Widgets: []GChatWidgets{
			{
				TextParagraph: GChatText{
					Text: "<font color=#848484><strong>" + message + "</strong></font>",
				},
			},
		},
	})
	for _, f := range fields {
		facts = append(facts, &GChatSections{
			Widgets: []GChatWidgets{
				{
					TextParagraph: GChatText{
						Text: f.Name + "<br><font color=#848484><strong>" + f.Value + "</strong></font>",
					},
				},
			},
		})
	}

	payload := GChatPayload{
		Cards: []GChatCards{
			{
				Header: GChatHeader{
					Title:    "Flagger",
					SubTitle: fmt.Sprintf("%s.%s", workload, namespace),
					ImageUrl: "https://flagger.app/favicon.png",
				},
				Sections: facts,
			},
		},
	}

	err := postMessage(s.URL, s.ProxyURL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}

	return nil
}
