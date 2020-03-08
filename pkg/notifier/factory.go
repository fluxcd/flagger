package notifier

import "fmt"

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

func (f Factory) Notifier(provider string) (Interface, error) {
	switch provider {
	case "slack":
		return NewSlack(f.URL, f.Username, f.Channel)
	case "discord":
		return NewDiscord(f.URL, f.Username, f.Channel)
	case "rocket":
		return NewRocket(f.URL, f.Username, f.Channel)
	case "msteams":
		return NewMSTeams(f.URL)
	}

	return nil, fmt.Errorf("provider %s not supported", provider)
}
