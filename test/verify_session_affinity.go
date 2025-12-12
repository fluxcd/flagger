package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// channel for canary cookie
var cc = make(chan string, 1)

// channel for primary cookie
var pc = make(chan string, 1)
var timeout = time.Second * 10

func main() {
	url := os.Getenv("URL")
	host := os.Getenv("HOST")
	canaryVersion := os.Getenv("CANARY_VERSION")
	canaryCookieName := os.Getenv("CANARY_COOKIE_NAME")
	primaryVersion := os.Getenv("PRIMARY_VERSION")
	primaryCookieName := os.Getenv("PRIMARY_COOKIE_NAME")

	go tryUntilWorkloadIsHit(url, host, canaryVersion, canaryCookieName, cc)
	go tryUntilWorkloadIsHit(url, host, primaryVersion, primaryCookieName, pc)

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go verifySessionAffinity(cc, wg, url, host, primaryVersion)
	go verifySessionAffinity(pc, wg, url, host, canaryVersion)

	wg.Wait()
	log.Println("✔ successfully verified session affinity")
}

func verifySessionAffinity(cc chan string, wg *sync.WaitGroup, url, host, wrongVersion string) {
	select {
	// If we receive a cookie, then try to verify that we are always routed to the
	// Canary deployment based on the cookie.
	case cookie := <-cc:
		for i := 0; i < 5; i++ {
			headers := map[string]string{
				"Cookie": cookie,
			}
			body, _, err := sendRequest(url, host, headers)
			if err != nil {
				log.Fatalf("failed to send request to verify cookie based routing: %v", err)
			}
			if strings.Contains(body, wrongVersion) {
				log.Fatalf("received response from the wrong deployment")
			}
		}
		wg.Done()
	case <-time.After(timeout):
		log.Fatal("timed out waiting for workload hit")
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	return string(body), resp.Cookies(), nil
}

// tryUntilWorkloadIsHit is a recursive function that tries to send request and
// either sends the cookie back to the main thread (if received) or re-sends
// the request.
func tryUntilWorkloadIsHit(url, host, version, cookieName string, c chan string) {
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

	tryUntilWorkloadIsHit(url, host, version, cookieName, c)
}
