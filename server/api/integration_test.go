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

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
	"github.com/3tty0n/mattermost-saved-links/server/savedlinks"
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

func createSavedLinkIntegration(t *testing.T, th *testhelper.TestHelper, client *model.Client4, body map[string]any) *savedlinkmodel.SavedLink {
	t.Helper()
	response, payload := pluginRequest(t, th, client, http.MethodPost, "/saved-links", body)
	require.Equal(t, http.StatusCreated, response.StatusCode, string(payload))
	var savedLink savedlinkmodel.SavedLink
	require.NoError(t, json.Unmarshal(payload, &savedLink))
	return &savedLink
}

func createFolderIntegration(t *testing.T, th *testhelper.TestHelper, client *model.Client4, body map[string]any) *savedlinkmodel.Folder {
	t.Helper()
	response, payload := pluginRequest(t, th, client, http.MethodPost, "/folders", body)
	require.Equal(t, http.StatusCreated, response.StatusCode, string(payload))
	var folder savedlinkmodel.Folder
	require.NoError(t, json.Unmarshal(payload, &folder))
	return &folder
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

func TestSavedLinkIntegration(t *testing.T) {
	th := testhelper.Setup(t)

	savedLink := createSavedLinkIntegration(t, th, th.Client, map[string]any{
		"channelId": th.Channel.Id,
		"url":       "https://example.com/docs",
		"name":      "Docs",
		"tags":      []string{"integration"},
	})
	assert.Equal(t, "Docs", savedLink.Name)
	assert.Equal(t, th.Team.Id, savedLink.TeamID)
	assert.Equal(t, th.Channel.Id, savedLink.ChannelID)

	response, payload := pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links?scope=channel&channel_id="+th.Channel.Id+"&q=docs", nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var list savedlinks.ListResult
	require.NoError(t, json.Unmarshal(payload, &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, savedLink.ID, list.Items[0].ID)

	response, payload = pluginRequest(t, th, th.Client, http.MethodPost, "/saved-links/"+savedLink.ID, map[string]any{"name": "Renamed"})
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var updated savedlinkmodel.SavedLink
	require.NoError(t, json.Unmarshal(payload, &updated))
	assert.Equal(t, "Renamed", updated.Name)

	restartPlugin(t, th)
	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links/"+savedLink.ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var restored savedlinkmodel.SavedLink
	require.NoError(t, json.Unmarshal(payload, &restored))
	assert.Equal(t, "Renamed", restored.Name)

	response, _ = pluginRequest(t, th, th.Client, http.MethodDelete, "/saved-links/"+savedLink.ID, nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links/"+savedLink.ID, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode, string(payload))
}

func TestFolderIntegration(t *testing.T) {
	th := testhelper.Setup(t)

	folder := createFolderIntegration(t, th, th.Client, map[string]any{
		"channelId": th.Channel.Id,
		"name":      "Work",
	})
	assert.Equal(t, th.Channel.Id, folder.ChannelID)

	// A URL is enough: the plugin derives the name and accepts the folder.
	savedLink := createSavedLinkIntegration(t, th, th.Client, map[string]any{
		"channelId": th.Channel.Id,
		"folderId":  folder.ID,
		"url":       "https://example.com/getting-started",
	})
	assert.Equal(t, "getting started", savedLink.Name)
	assert.Equal(t, folder.ID, savedLink.FolderID)

	second := createSavedLinkIntegration(t, th, th.Client, map[string]any{
		"channelId": th.Channel.Id,
		"folderId":  folder.ID,
		"url":       "https://example.com/second",
		"name":      "Second link",
	})

	response, payload := pluginRequest(t, th, th.Client, http.MethodGet, "/folders?channel_id="+th.Channel.Id, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var folders savedlinks.FolderListResult
	require.NoError(t, json.Unmarshal(payload, &folders))
	require.Len(t, folders.Items, 1)
	assert.Equal(t, 2, folders.Items[0].SavedLinkCount)

	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links?scope=channel&channel_id="+th.Channel.Id+"&folder_id="+folder.ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var list savedlinks.ListResult
	require.NoError(t, json.Unmarshal(payload, &list))
	require.Len(t, list.Items, 2)

	response, payload = pluginRequest(t, th, th.Client, http.MethodPost, "/saved-links/reorder", map[string]any{
		"savedLinkIds": []string{second.ID, savedLink.ID},
	})
	require.Equal(t, http.StatusNoContent, response.StatusCode, string(payload))

	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links?scope=channel&channel_id="+th.Channel.Id+"&folder_id="+folder.ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	require.NoError(t, json.Unmarshal(payload, &list))
	require.Len(t, list.Items, 2)
	assert.Equal(t, second.ID, list.Items[0].ID)
	assert.Equal(t, savedLink.ID, list.Items[1].ID)

	response, payload = pluginRequest(t, th, th.Client, http.MethodPost, "/folders/"+folder.ID, map[string]any{"name": "Reference"})
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var renamed savedlinkmodel.Folder
	require.NoError(t, json.Unmarshal(payload, &renamed))
	assert.Equal(t, "Reference", renamed.Name)

	response, _ = pluginRequest(t, th, th.Client, http.MethodDelete, "/folders/"+folder.ID, nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links/"+savedLink.ID, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var restored savedlinkmodel.SavedLink
	require.NoError(t, json.Unmarshal(payload, &restored))
	assert.Empty(t, restored.FolderID)
}

func TestPrivateChannelSavedLinkIsolation(t *testing.T) {
	th := testhelper.Setup(t)
	privateChannel := th.CreateChannel(model.ChannelTypePrivate)

	privateSavedLink := createSavedLinkIntegration(t, th, th.AdminClient, map[string]any{
		"channelId": privateChannel.Id,
		"url":       "https://example.com/secret",
		"name":      "Secret",
	})

	response, payload := pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links/"+privateSavedLink.ID, nil)
	assert.Equal(t, http.StatusNotFound, response.StatusCode, string(payload))

	response, payload = pluginRequest(t, th, th.Client, http.MethodGet, "/saved-links?scope=team&team_id="+th.Team.Id, nil)
	require.Equal(t, http.StatusOK, response.StatusCode, string(payload))
	var list savedlinks.ListResult
	require.NoError(t, json.Unmarshal(payload, &list))
	for _, item := range list.Items {
		assert.NotEqual(t, privateSavedLink.ID, item.ID)
	}

	response, _ = pluginRequest(t, th, nil, http.MethodGet, "/saved-links?scope=all", nil)
	assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
}
