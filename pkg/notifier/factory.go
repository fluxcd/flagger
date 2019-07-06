package notifier

import (
	"strings"
)

type Factory struct {
	URL      string
	Username string
	Channel  string
}

func NewFactory(URL string, username string, channel string) *Factory {
	return &Factory{
		URL:      URL,
		Channel:  channel,
		Username: username,
	}
}

func (f Factory) Notifier() (Interface, error) {
	switch {
	case strings.Contains(f.URL, "slack.com"):
		return NewSlack(f.URL, f.Username, f.Channel)
	}

	return nil, nil
}
