package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlack_Post(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload = SlackPayload{}
		err = json.Unmarshal(b, &payload)

		if payload.Attachments[0].AuthorName != "podinfo.test" {
			t.Fatal("wrong author name")
		}
	}))
	defer ts.Close()

	slack, err := NewSlack(ts.URL, "test", "test")
	if err != nil {
		t.Fatal(err)
	}

	err = slack.Post("podinfo", "test", "test", nil, true)
	if err != nil {
		t.Fatal(err)
	}
}
