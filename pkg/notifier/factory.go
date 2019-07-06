package notifier

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
	case provider == "msteams":
		return NewMSTeams(f.URL)
	}

	return nil, nil
}
