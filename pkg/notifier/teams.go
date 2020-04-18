package notifier

import (
	"fmt"
	"net/url"
)

// MS Teams holds the incoming webhook URL
type MSTeams struct {
	URL string
}

// MSTeamsPayload holds the message card data
type MSTeamsPayload struct {
	Type       string           `json:"@type"`
	Context    string           `json:"@context"`
	ThemeColor string           `json:"themeColor"`
	Summary    string           `json:"summary"`
	Sections   []MSTeamsSection `json:"sections"`
}

// MSTeamsSection holds the canary analysis result
type MSTeamsSection struct {
	ActivityTitle    string         `json:"activityTitle"`
	ActivitySubtitle string         `json:"activitySubtitle"`
	Facts            []MSTeamsField `json:"facts"`
}

type MSTeamsField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// NewMSTeams validates the MS Teams URL and returns a MSTeams object
func NewMSTeams(hookURL string) (*MSTeams, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid MS Teams webhook URL %s", hookURL)
	}

	return &MSTeams{
		URL: hookURL,
	}, nil
}

// Post MS Teams message
func (s *MSTeams) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	facts := make([]MSTeamsField, 0, len(fields))
	for _, f := range fields {
		facts = append(facts, MSTeamsField(f))
	}

	payload := MSTeamsPayload{
		Type:       "MessageCard",
		Context:    "http://schema.org/extensions",
		ThemeColor: "0076D7",
		Summary:    fmt.Sprintf("%s.%s", workload, namespace),
		Sections: []MSTeamsSection{
			{
				ActivityTitle:    message,
				ActivitySubtitle: fmt.Sprintf("%s.%s", workload, namespace),
				Facts:            facts,
			},
		},
	}

	if severity == "error" {
		payload.ThemeColor = "FF0000"
	}

	err := postMessage(s.URL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}

	return nil
}
