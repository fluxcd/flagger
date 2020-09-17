package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDingDing_Post(t *testing.T) {
	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		var payload = DingDingPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)
		// require.Equal(t, "podinfo.test", payload.Text.Content)
		// require.Equal(t, len(fields), len(payload.Attachments[0].Fields))
	}))
	defer ts.Close()

	url := "https://oapi.dingtalk.com/robot/send?access_token=135216fa4875c302b6d1dd98ab3fc23b0beb721de31feb7e40801837b513a41e"
	dingDing, err := NewDingDing(url, "13752557129,13540484322", "text")
	require.NoError(t, err)

	err = dingDing.Post("podinfo", "test", "test", fields, "error")
	require.NoError(t, err)

}
