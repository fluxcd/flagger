package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRocket_Post(t *testing.T) {
	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		var payload = SlackPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)
		require.Equal(t, "podinfo.test", payload.Attachments[0].AuthorName)
		require.Equal(t, len(fields), len(payload.Attachments[0].Fields))
	}))
	defer ts.Close()

	rocket, err := NewRocket(ts.URL, "test", "test")
	require.NoError(t, err)

	err = rocket.Post("podinfo", "test", "test", fields, "error")
	require.NoError(t, err)

}
