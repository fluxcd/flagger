package notifier

import (
	"fmt"
	"net/url"
)

// DingTalk holds the incoming webhook URL
type DingTalk struct {
	URL string
}

// NewDingTalk validates the DingTalk URL and returns a DingTalk object
func NewDingTalk(hookURL string) (*DingTalk, error) {
	_, err := url.ParseRequestURI(hookURL)
	if err != nil {
		return nil, fmt.Errorf("invalid DingTalk webhook URL %s", hookURL)
	}

	return &DingTalk{
		URL: hookURL,
	}, nil
}

//Text
type Text struct {
	Content string `json:"content"`
}

//At
type At struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

//Payload
type DingTalkPayload struct {
	MessageType string `json:"msgtype"`
	Text        Text   `json:"text"`
	At          At     `json:"at"`
}

/**
* Post DingTalk message
* In the DingTalk Webhook API, the field "Field" does not exist, so you can use it as a parameter to the incoming phone number
* for example:
* fields := []Field{
* 		{Name: "user1",Value: "13512345678"},
*   	{Name: "user2",Value: "13612345678"},
* }
*
 */
func (d *DingTalk) Post(workload string, namespace string, message string, fields []Field, severity string) error {
	atMobiles := make([]string, 0)
	//Convert fileds
	for _, f := range fields {
		if f.Value != "" {
			atMobiles = append(atMobiles, f.Value)
		}
	}

	payload := DingTalkPayload{
		MessageType: "text",
		Text: Text{
			Content: fmt.Sprintf("%s.%s,%s", workload, namespace, message),
		},
		At: At{
			AtMobiles: atMobiles,
			IsAtAll:   false,
		},
	}

	// If severity equals "error", @All users
	if severity == "error" {
		payload.At.IsAtAll = true
	}

	err := postMessage(d.URL, payload)
	if err != nil {
		return fmt.Errorf("postMessage failed: %w", err)
	}
	return nil
}
