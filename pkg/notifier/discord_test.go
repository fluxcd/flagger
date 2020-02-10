package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscord_Post(t *testing.T) {
	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

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

		if len(payload.Attachments[0].Fields) != len(fields) {
			t.Fatal("wrong facts")
		}
	}))
	defer ts.Close()

	discord, err := NewDiscord(ts.URL, "test", "test")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(discord.URL, "/slack") {
		t.Error("Invalid Discord URL, expected to have /slack prefix")
	}

	err = discord.Post("podinfo", "test", "test", fields, "warn")
	if err != nil {
		t.Fatal(err)
	}
}
