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
)

type Factory struct {
	URL      string
	ProxyURL string
	Username string
	Channel  string
}

func NewFactory(url string, proxy string, username string, channel string) *Factory {
	return &Factory{
		URL:      url,
		ProxyURL: proxy,
		Channel:  channel,
		Username: username,
	}
}

func (f Factory) Notifier(provider string) (Interface, error) {
	if f.URL == "" {
		return &NopNotifier{}, nil
	}

	var n Interface
	var err error
	switch provider {
	case "slack":
		n, err = NewSlack(f.URL, f.ProxyURL, f.Username, f.Channel)
	case "discord":
		n, err = NewDiscord(f.URL, f.ProxyURL, f.Username, f.Channel)
	case "rocket":
		n, err = NewRocket(f.URL, f.ProxyURL, f.Username, f.Channel)
	case "msteams":
		n, err = NewMSTeams(f.URL, f.ProxyURL)
	case "gchat":
		n, err = NewGChat(f.URL, f.ProxyURL)
	default:
		err = fmt.Errorf("provider %s not supported", provider)
	}

	if err != nil {
		n = &NopNotifier{}
	}
	return n, err
}
