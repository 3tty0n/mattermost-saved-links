package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/3tty0n/mattermost-bookmarks/server/bookmarks"
	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

func pluginRequest(t *testing.T, th *testhelper.TestHelper, client *model.Client4, method, path string, body any) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, th.ServerURL+"/plugins/"+testhelper.PluginID()+"/api/v1"+path, reader)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if client != nil {
		request.Header.Set(model.HeaderAuth, "Bearer "+client.AuthToken)
	}

	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response, payload
}

func createBookmarkIntegration(t *testing.T, th *testhelper.TestHelper, client *model.Client4, body map[string]any) *bookmarkmodel.Bookmark {
	t.Helper()
	response, payload := pluginRequest(t, th, client, http.MethodPost, "/bookmarks", body)
	require.Equal(t, http.StatusCreated, response.StatusCode, string(payload))
	var bookmark bookmarkmodel.Bookmark
	require.NoError(t, json.Unmarshal(payload, &bookmark))
	return &bookmark
}

func waitForPluginRunning(t *testing.T, th *testhelper.TestHelper) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		statuses, _, err := th.AdminClient.GetPluginStatuses(t.Context())
		if err == nil {
			for _, status := range statuses {
				if status.PluginId == testhelper.PluginID() && status.State == model.PluginStateRunning {
					return
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("plugin did not return to the running state")
}

func restartPlugin(t *testing.T, th *testhelper.TestHelper) {
	t.Helper()
	_, err := th.AdminClient.DisablePlugin(t.Context(), testhelper.PluginID())
	require.NoError(t, err)
	_, err = th.AdminClient.EnablePlugin(t.Context(), testhelper.PluginID())
	require.NoError(t, err)
	waitForPluginRunning(t, th)
}

func TestBookmarkIntegration(t *testing.T) {
	th := testhelper.Setup(t)

	bookmark := createBookmarkIntegration(t, th, th.Client, map[string]any{
		"channelId":   th.Channel.Id,
		"url":         "https://example.com/docs",
		"title":       "Docs",
		"description": "Reference material",
		"tags":        []string{"integration"},
	})
	assert.Equal(t, "Docs", bookmark.Title)
	assert.Equal(t, th.Team.Id, bookmark.TeamID)
	assert.Equal(t, th.Channel.Id, bookmark.ChannelID)

	response, payload := pluginRequest(t, th, th.Client, http.MethodGet, "/bookmarks?scope=channel&channel_id="+th.Channel.Id+"&q=docs", nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var list bookmarks.ListResult
	require.NoError(t, json.Unmarshal(payload, &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, bookmark.ID, list.Items[0].ID)

	response, payload = pluginRequest(t, th, th.Client, http.MethodPost, "/bookmarks/"+bookmark.ID, map[string]any{"title": "Renamed"})
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var updated bookmarkmodel.Bookmark
	require.NoError(t, json.Unmarshal(payload, &updated))
	assert.Equal(t, "Renamed", updated.Title)

	restartPlugin(t, th)
	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/bookmarks/"+bookmark.ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var restored bookmarkmodel.Bookmark
	require.NoError(t, json.Unmarshal(payload, &restored))
	assert.Equal(t, "Renamed", restored.Title)

	response, _ = pluginRequest(t, th, th.Client, http.MethodDelete, "/bookmarks/"+bookmark.ID, nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/bookmarks/"+bookmark.ID, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode, string(payload))
}

func TestPrivateChannelBookmarkIsolation(t *testing.T) {
	th := testhelper.Setup(t)
	privateChannel := th.CreateChannel(model.ChannelTypePrivate)

	privateBookmark := createBookmarkIntegration(t, th, th.AdminClient, map[string]any{
		"channelId": privateChannel.Id,
		"url":       "https://example.com/secret",
		"title":     "Secret",
	})

	response, payload := pluginRequest(t, th, th.Client, http.MethodGet, "/bookmarks/"+privateBookmark.ID, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode, string(payload))

	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/bookmarks?scope=team&team_id="+th.Team.Id, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var list bookmarks.ListResult
	require.NoError(t, json.Unmarshal(payload, &list))
	for _, item := range list.Items {
		assert.NotEqual(t, privateBookmark.ID, item.ID)
	}

	response, _ = pluginRequest(t, th, nil, http.MethodGet, "/bookmarks?scope=all", nil)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
}
