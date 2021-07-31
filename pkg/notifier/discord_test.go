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
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var payload = SlackPayload{}
		err = json.Unmarshal(b, &payload)
		require.NoError(t, err)
		require.Equal(t, "podinfo.test", payload.Attachments[0].AuthorName)
		require.Equal(t, len(fields), len(payload.Attachments[0].Fields))
	}))
	defer ts.Close()

	discord, err := NewDiscord(ts.URL, "", "test", "test")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(discord.URL, "/slack"))

	err = discord.Post("podinfo", "test", "test", fields, "warn")
	require.NoError(t, err)
}
