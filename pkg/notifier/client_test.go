package notifier

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_postMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		var payload = make(map[string]string)
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)

		require.Equal(t, "success", payload["status"])
	}))
	defer ts.Close()

	err := postMessage(ts.URL, map[string]string{"status": "success"})
	require.NoError(t, err)
}
