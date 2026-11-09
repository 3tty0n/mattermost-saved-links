package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/3tty0n/mattermost-saved-links/server/access"
	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
	"github.com/3tty0n/mattermost-saved-links/server/savedlinks"
	"github.com/3tty0n/mattermost-saved-links/server/store/kvstore"
	"github.com/3tty0n/mattermost-saved-links/server/testutil"
)

type noopPublisher struct{}

func (noopPublisher) SavedLinkChanged(string, *savedlinkmodel.SavedLink) {}
func (noopPublisher) FolderChanged(string, *savedlinkmodel.Folder)       {}

type apiTestEnv struct {
	handler *Handler
	api     *testutil.API
	store   kvstore.Store
	service *savedlinks.Service
}

func newAPITestEnv(t *testing.T) *apiTestEnv {
	t.Helper()
	api := testutil.NewAPI()
	t.Cleanup(func() { api.AssertExpectations(t) })
	client := pluginapi.NewClient(api, &plugintest.Driver{})
	checker := access.NewChecker(client)
	store := kvstore.NewStoreWithKV(kvstore.NewMemoryKV())
	service := savedlinks.NewService(client, store, checker, noopPublisher{}, savedlinks.WithClock(func() int64 { return 1000 }))
	return &apiTestEnv{
		handler: NewHandler(api, client, checker, service),
		api:     api,
		store:   store,
		service: service,
	}
}

func (e *apiTestEnv) serve(method, path, body, userID string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if userID != "" {
		r.Header.Set("Mattermost-User-ID", userID)
	}
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(nil, w, r)
	return w
}

func decodeResponse[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return out
}

func allowChannel(api *testutil.API, userID, channelID, teamID string) {
	api.On("GetChannel", channelID).Return(&model.Channel{Id: channelID, TeamId: teamID, Type: model.ChannelTypeOpen}, nil).Maybe()
	api.On("HasPermissionToChannel", userID, channelID, model.PermissionReadChannel).Return(true).Maybe()
}

func denyAdmin(api *testutil.API, userID string) {
	api.On("HasPermissionTo", userID, model.PermissionManageSystem).Return(false).Maybe()
}

func TestAllRoutesRejectUnauthenticatedRequests(t *testing.T) {
	env := newAPITestEnv(t)
	id := model.NewId()
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/saved-links"},
		{http.MethodPost, "/api/v1/saved-links"},
		{http.MethodPost, "/api/v1/saved-links/reorder"},
		{http.MethodGet, "/api/v1/saved-links/" + id},
		{http.MethodPatch, "/api/v1/saved-links/" + id},
		{http.MethodDelete, "/api/v1/saved-links/" + id},
		{http.MethodGet, "/api/v1/folders"},
		{http.MethodPost, "/api/v1/folders"},
		{http.MethodGet, "/api/v1/folders/" + id},
		{http.MethodPatch, "/api/v1/folders/" + id},
		{http.MethodDelete, "/api/v1/folders/" + id},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			w := env.serve(route.method, route.path, "", "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestSavedLinkAPILifecycle(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID := model.NewId(), model.NewId()
	creatorID, strangerID := model.NewId(), model.NewId()
	allowChannel(env.api, creatorID, channelID, teamID)
	denyAdmin(env.api, creatorID)
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(false).Maybe()

	createBody := `{"channelId":"` + channelID + `","url":"https://example.com/docs","name":"Docs","tags":["work"]}`
	w := env.serve(http.MethodPost, "/api/v1/saved-links", createBody, creatorID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeResponse[savedlinkmodel.SavedLink](t, w)
	assert.Equal(t, "https://example.com/docs", created.URL)
	assert.Equal(t, "Docs", created.Name)

	w = env.serve(http.MethodGet, "/api/v1/saved-links/"+created.ID, "", creatorID)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, created.ID, decodeResponse[savedlinkmodel.SavedLink](t, w).ID)

	w = env.serve(http.MethodGet, "/api/v1/saved-links/"+created.ID, "", strangerID)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID+"&q=docs", "", creatorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decodeResponse[savedlinks.ListResult](t, w)
	assert.Equal(t, 1, list.Total)

	w = env.serve(http.MethodPost, "/api/v1/saved-links/"+created.ID, `{"name":"Renamed"}`, creatorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Renamed", decodeResponse[savedlinkmodel.SavedLink](t, w).Name)

	w = env.serve(http.MethodDelete, "/api/v1/saved-links/"+created.ID, "", creatorID)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = env.serve(http.MethodGet, "/api/v1/saved-links/"+created.ID, "", creatorID)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSavedLinkAPIURLOnlyAndFolders(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID := model.NewId(), model.NewId()
	actorID := model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	denyAdmin(env.api, actorID)

	// A URL is enough to create a saved link.
	w := env.serve(http.MethodPost, "/api/v1/saved-links", `{"channelId":"`+channelID+`","url":"https://example.com/getting-started"}`, actorID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeResponse[savedlinkmodel.SavedLink](t, w)
	assert.Equal(t, "getting started", created.Name)
	assert.Empty(t, created.FolderID)

	w = env.serve(http.MethodPost, "/api/v1/folders", `{"channelId":"`+channelID+`","name":"Work"}`, actorID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	folder := decodeResponse[savedlinkmodel.Folder](t, w)
	assert.Equal(t, "Work", folder.Name)

	w = env.serve(http.MethodGet, "/api/v1/folders?channel_id="+channelID, "", actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	folders := decodeResponse[savedlinks.FolderListResult](t, w)
	require.Len(t, folders.Items, 1)
	assert.Equal(t, 0, folders.Items[0].SavedLinkCount)

	w = env.serve(http.MethodPatch, "/api/v1/saved-links/"+created.ID, `{"folderId":"`+folder.ID+`"}`, actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	updated := decodeResponse[savedlinkmodel.SavedLink](t, w)
	assert.Equal(t, folder.ID, updated.FolderID)

	w = env.serve(http.MethodGet, "/api/v1/folders?channel_id="+channelID, "", actorID)
	require.Equal(t, http.StatusOK, w.Code)
	folders = decodeResponse[savedlinks.FolderListResult](t, w)
	require.Len(t, folders.Items, 1)
	assert.Equal(t, 1, folders.Items[0].SavedLinkCount)

	w = env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID+"&folder_id="+folder.ID, "", actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decodeResponse[savedlinks.ListResult](t, w)
	require.Len(t, list.Items, 1)
	assert.Equal(t, created.ID, list.Items[0].ID)

	w = env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID+"&folder_id=none", "", actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list = decodeResponse[savedlinks.ListResult](t, w)
	assert.Empty(t, list.Items)

	w = env.serve(http.MethodPost, "/api/v1/folders/"+folder.ID, `{"name":"Reference"}`, actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Reference", decodeResponse[savedlinkmodel.Folder](t, w).Name)

	w = env.serve(http.MethodDelete, "/api/v1/folders/"+folder.ID, "", actorID)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	w = env.serve(http.MethodGet, "/api/v1/saved-links/"+created.ID, "", actorID)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, decodeResponse[savedlinkmodel.SavedLink](t, w).FolderID)
}

func TestSavedLinkAPIReorder(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	create := func(name, url string) savedlinkmodel.SavedLink {
		body := `{"channelId":"` + channelID + `","url":"` + url + `","name":"` + name + `"}`
		w := env.serve(http.MethodPost, "/api/v1/saved-links", body, actorID)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return decodeResponse[savedlinkmodel.SavedLink](t, w)
	}
	first := create("First", "https://example.com/1")
	second := create("Second", "https://example.com/2")

	body := `{"savedLinkIds":["` + second.ID + `","` + first.ID + `"]}`
	w := env.serve(http.MethodPost, "/api/v1/saved-links/reorder", body, actorID)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	w = env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID, "", actorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decodeResponse[savedlinks.ListResult](t, w)
	require.Len(t, list.Items, 2)
	assert.Equal(t, second.ID, list.Items[0].ID)
	assert.Equal(t, first.ID, list.Items[1].ID)

	w = env.serve(http.MethodPost, "/api/v1/saved-links/reorder", `{"savedLinkIds":[]}`, actorID)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSavedLinkAPIValidationAndPermissions(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID := model.NewId(), model.NewId()
	actorID := model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	t.Run("invalid JSON", func(t *testing.T) {
		w := env.serve(http.MethodPost, "/api/v1/saved-links", `{"name":`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("unknown field", func(t *testing.T) {
		w := env.serve(http.MethodPost, "/api/v1/saved-links", `{"name":"x","bogus":true}`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("description is no longer accepted", func(t *testing.T) {
		body := `{"channelId":"` + channelID + `","url":"https://example.com","description":"x"}`
		w := env.serve(http.MethodPost, "/api/v1/saved-links", body, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("javascript URL", func(t *testing.T) {
		body := `{"channelId":"` + channelID + `","url":"javascript:alert(1)","name":"x"}`
		w := env.serve(http.MethodPost, "/api/v1/saved-links", body, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("credentials in URL", func(t *testing.T) {
		body := `{"channelId":"` + channelID + `","url":"https://user:pass@example.com","name":"x"}`
		w := env.serve(http.MethodPost, "/api/v1/saved-links", body, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("invalid ID", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/saved-links/not-an-id", "", actorID)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("invalid folder ID", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID+"&folder_id=bad", "", actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("empty patch", func(t *testing.T) {
		w := env.serve(http.MethodPatch, "/api/v1/saved-links/"+model.NewId(), `{}`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("empty folder patch", func(t *testing.T) {
		w := env.serve(http.MethodPatch, "/api/v1/folders/"+model.NewId(), `{}`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("team list forbidden", func(t *testing.T) {
		env.api.On("HasPermissionToTeam", actorID, teamID, model.PermissionViewTeam).Return(false).Maybe()
		w := env.serve(http.MethodGet, "/api/v1/saved-links?scope=team&team_id="+teamID, "", actorID)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("folder list needs a scope", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/folders", "", actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("invalid pagination", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/saved-links?scope=my&page=abc", "", actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAPISetsNoStoreAndContentType(t *testing.T) {
	env := newAPITestEnv(t)
	actorID := model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	w := env.serve(http.MethodGet, "/api/v1/saved-links?scope=channel&channel_id="+channelID, "", actorID)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}
