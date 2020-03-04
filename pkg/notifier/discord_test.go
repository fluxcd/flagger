package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscord_Post(t *testing.T) {
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

	discord, err := NewDiscord(ts.URL, "test", "test")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(discord.URL, "/slack"))

	err = discord.Post("podinfo", "test", "test", fields, "warn")
	require.NoError(t, err)
}
