package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

//Text
func TestDingTalkText_Post(t *testing.T) {
	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)
		var payload = DingTalkPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)

		require.Equal(t, "podinfo.test,test", payload.Text.Content)
		require.Equal(t, len(fields), len(payload.At.AtMobiles))
	}))
	defer ts.Close()

	dingTalk, err := NewDingTalk(ts.URL)
	require.NoError(t, err)

	err = dingTalk.Post("podinfo", "test", "test", fields, "info")
	require.NoError(t, err)
}
