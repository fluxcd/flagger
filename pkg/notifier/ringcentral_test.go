/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRingCentral_NewRingCentral(t *testing.T) {
	_, err := NewRingCentral("invalid-url", "")
	require.Error(t, err)

	ringCentral, err := NewRingCentral("http://localhost", "")
	require.NoError(t, err)
	require.Equal(t, "http://localhost", ringCentral.URL)
}

func TestRingCentral_Post(t *testing.T) {
	fields := []Field{
		{Name: "name1", Value: "value1"},
		{Name: "name2", Value: "value2"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var payload = RingCentralPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)
		require.Equal(t, "podinfo.test", payload.Activity)
		require.Equal(t, len(fields)+1, len(payload.Attachments[0].Body))
		require.Equal(t, "http://adaptivecards.io/schemas/adaptive-card.json", payload.Attachments[0].Schema)
		require.Equal(t, "AdaptiveCard", payload.Attachments[0].Type)
		require.Equal(t, "1.0", payload.Attachments[0].Version)
	}))
	defer ts.Close()

	ringCentral, err := NewRingCentral(ts.URL, "")
	require.NoError(t, err)

	err = ringCentral.Post("podinfo", "test", "test", fields, "info")
	require.NoError(t, err)
	err = ringCentral.Post("podinfo", "test", "test", fields, "error")
	require.NoError(t, err)
	err = ringCentral.Post("podinfo", "test", "test", fields, "warn")
	require.NoError(t, err)

}
