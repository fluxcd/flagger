package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTeams_Post(t *testing.T) {

	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)
		var payload = MSTeamsPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)

		require.Equal(t, "podinfo.test", payload.Sections[0].ActivitySubtitle)
		require.Equal(t, len(fields), len(payload.Sections[0].Facts))
	}))
	defer ts.Close()

	teams, err := NewMSTeams(ts.URL)
	require.NoError(t, err)

	err = teams.Post("podinfo", "test", "test", fields, "info")
	require.NoError(t, err)
}
