package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_postMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload = make(map[string]string)
		err = json.Unmarshal(b, &payload)

		if payload["status"] != "success" {
			t.Fatal("wrong payload")
		}
	}))
	defer ts.Close()

	err := postMessage(ts.URL, map[string]string{"status": "success"})
	if err != nil {
		t.Fatal(err)
	}
}
