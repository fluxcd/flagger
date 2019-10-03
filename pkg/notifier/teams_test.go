package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTeams_Post(t *testing.T) {

	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

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
		if len(payload.Sections[0].Facts) != len(fields) {
			t.Fatal("wrong facts")
		}
	}))
	defer ts.Close()

	teams, err := NewMSTeams(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	err = teams.Post("podinfo", "test", "test", fields, true)
	if err != nil {
		t.Fatal(err)
	}
}
