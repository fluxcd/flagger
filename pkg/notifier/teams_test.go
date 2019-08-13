package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTeams_Post(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload = MSTeamsPayload{}
		err = json.Unmarshal(b, &payload)

		if payload.Sections[0].ActivitySubtitle != "podinfo.test" {
			t.Fatal("wrong activity subtitle")
		}
	}))
	defer ts.Close()

	teams, err := NewMSTeams(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = teams.Post("podinfo", "test", "test", nil, true)
	if err != nil {
		t.Fatal(err)
	}
}
