package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var c = make(chan string, 1)
var mu sync.Mutex
var try = true
var timeout = time.Second * 10

func main() {
	url := os.Getenv("URL")
	host := os.Getenv("HOST")
	version := os.Getenv("VERSION")
	cookieName := os.Getenv("COOKIE_NAME")

	// Generate traffic
	for i := 0; i < 10; i++ {
		go tryUntilCanaryIsHit(url, host, version, cookieName)
	}

	select {
	// If we receive a cookie, then try to verify that we are always routed to the
	// Canary deployment based on the cookie.
	case cookie := <-c:
		mu.Lock()
		try = false
		mu.Unlock()

		for i := 0; i < 5; i++ {
			headers := map[string]string{
				"Cookie": cookie,
			}
			body, _, err := sendRequest(url, host, headers)
			if err != nil {
				log.Fatalf("failed to send request to verify cookie based routing: %v", err)
			}
			if !strings.Contains(body, version) {
				log.Fatalf("received response from primary deployment instead of canary deployment")
			}
		}

		log.Println("✔ successfully verified session affinity")
	case <-time.After(timeout):
		log.Fatal("timed out waiting for canary hit")
	}
}

// sendRequest sends a request to the URL with the provided host and headers.
// It returns the response body and cookies or an error.
func sendRequest(url, host string, headers map[string]string) (string, []*http.Cookie, error) {
	client := http.DefaultClient
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", nil, err
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}
	req.Host = host

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	return string(body), resp.Cookies(), nil
}

// tryUntilCanaryIsHit is a recursive function that tries to send request and
// either sends the cookie back to the main thread (if received) or re-sends
// the request.
func tryUntilCanaryIsHit(url, host, version, cookieName string) {
	mu.Lock()
	if !try {
		mu.Unlock()
		return
	}
	mu.Unlock()

	body, cookies, err := sendRequest(url, host, nil)
	if err != nil {
		log.Printf("warning: failed to send request: %s", err)
		return
	}
	if strings.Contains(body, version) {
		if cookies[0].Name == cookieName {
			c <- fmt.Sprintf("%s=%s", cookies[0].Name, cookies[0].Value)
			return
		}
	}

	tryUntilCanaryIsHit(url, host, version, cookieName)
	return
}
