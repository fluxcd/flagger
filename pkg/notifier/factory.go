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
	switch {
	case provider == "slack":
		return NewSlack(f.URL, f.Username, f.Channel)
	case provider == "discord":
		return NewDiscord(f.URL, f.Username, f.Channel)
	case provider == "rocket":
		return NewRocket(f.URL, f.Username, f.Channel)
	case provider == "msteams":
		return NewMSTeams(f.URL)
	}

	return nil, fmt.Errorf("provider %s not supported", provider)
}
