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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

func postMessage(address string, proxy string, payload interface{}) error {
	var httpClient = &http.Client{}

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return fmt.Errorf("unable to parse proxy URL '%s', error: %w", proxy, err)
		}
		httpClient.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling notification payload failed: %w", err)
	}

	b := bytes.NewBuffer(data)

	req, err := http.NewRequest("POST", address, b)
	if err != nil {
		return fmt.Errorf("http.NewRequest failed: %w", err)
	}
	req.Header.Set("Content-type", "application/json")

	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()

	res, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("sending notification failed: %w", err)
	}

	defer res.Body.Close()
	statusCode := res.StatusCode
	if statusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("sending notification failed: %s", string(body))
	}

	return nil
}
