package notifier

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Slack holds the hook URL
type Slack struct {
	URL       string
	Username  string
	Channel   string
	IconEmoji string
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
	Color      string   `json:"color"`
	AuthorName string   `json:"author_name"`
	Text       string   `json:"text"`
	MrkdwnIn   []string `json:"mrkdwn_in"`
}

// NewSlack validates the Slack URL and returns a Slack object
func NewSlack(hookURL string, username string, channel string) (*Slack, error) {
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
		Channel:   channel,
		URL:       hookURL,
		Username:  username,
		IconEmoji: ":rocket:",
	}, nil
}

// Post Slack message
func (s *Slack) Post(workload string, namespace string, message string, warn bool) error {
	payload := SlackPayload{
		Channel:  s.Channel,
		Username: s.Username,
	}

	color := "good"
	if warn {
		color = "danger"
	}

	a := SlackAttachment{
		Color:      color,
		AuthorName: fmt.Sprintf("%s.%s", workload, namespace),
		Text:       message,
		MrkdwnIn:   []string{"text"},
	}

	payload.Attachments = []SlackAttachment{a}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling slack payload failed %v", err)
	}

	b := bytes.NewBuffer(data)

	if res, err := http.Post(s.URL, "application/json", b); err != nil {
		return fmt.Errorf("sending data to slack failed %v", err)
	} else {
		defer res.Body.Close()
		statusCode := res.StatusCode
		if statusCode != 200 {
			body, _ := ioutil.ReadAll(res.Body)
			return fmt.Errorf("sending data to slack failed %v", string(body))
		}
	}

	return nil
}
