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

	"github.com/3tty0n/mattermost-bookmarks/server/access"
	"github.com/3tty0n/mattermost-bookmarks/server/bookmarks"
	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
	"github.com/3tty0n/mattermost-bookmarks/server/store/kvstore"
	"github.com/3tty0n/mattermost-bookmarks/server/testutil"
)

type noopPublisher struct{}

func (noopPublisher) BookmarkChanged(string, *bookmarkmodel.Bookmark) {}

type apiTestEnv struct {
	handler *Handler
	api     *testutil.API
	store   kvstore.Store
	service *bookmarks.Service
}

func newAPITestEnv(t *testing.T) *apiTestEnv {
	t.Helper()
	api := testutil.NewAPI()
	t.Cleanup(func() { api.AssertExpectations(t) })
	client := pluginapi.NewClient(api, &plugintest.Driver{})
	checker := access.NewChecker(client)
	store := kvstore.NewStoreWithKV(kvstore.NewMemoryKV())
	service := bookmarks.NewService(client, store, checker, noopPublisher{}, bookmarks.WithClock(func() int64 { return 1000 }))
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
		{http.MethodGet, "/api/v1/bookmarks"},
		{http.MethodPost, "/api/v1/bookmarks"},
		{http.MethodGet, "/api/v1/bookmarks/" + id},
		{http.MethodPatch, "/api/v1/bookmarks/" + id},
		{http.MethodDelete, "/api/v1/bookmarks/" + id},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			w := env.serve(route.method, route.path, "", "")
			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

func TestBookmarkAPILifecycle(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID := model.NewId(), model.NewId()
	creatorID, strangerID := model.NewId(), model.NewId()
	allowChannel(env.api, creatorID, channelID, teamID)
	denyAdmin(env.api, creatorID)
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(false).Maybe()

	createBody := `{"channelId":"` + channelID + `","url":"https://example.com/docs","title":"Docs","tags":["work"]}`
	w := env.serve(http.MethodPost, "/api/v1/bookmarks", createBody, creatorID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeResponse[bookmarkmodel.Bookmark](t, w)
	assert.Equal(t, "https://example.com/docs", created.URL)

	w = env.serve(http.MethodGet, "/api/v1/bookmarks/"+created.ID, "", creatorID)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, created.ID, decodeResponse[bookmarkmodel.Bookmark](t, w).ID)

	w = env.serve(http.MethodGet, "/api/v1/bookmarks/"+created.ID, "", strangerID)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = env.serve(http.MethodGet, "/api/v1/bookmarks?scope=channel&channel_id="+channelID+"&q=docs", "", creatorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decodeResponse[bookmarks.ListResult](t, w)
	assert.Equal(t, 1, list.Total)

	w = env.serve(http.MethodPost, "/api/v1/bookmarks/"+created.ID, `{"title":"Renamed"}`, creatorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Renamed", decodeResponse[bookmarkmodel.Bookmark](t, w).Title)

	w = env.serve(http.MethodPatch, "/api/v1/bookmarks/"+created.ID, `{"description":"notes"}`, creatorID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "notes", decodeResponse[bookmarkmodel.Bookmark](t, w).Description)

	w = env.serve(http.MethodDelete, "/api/v1/bookmarks/"+created.ID, "", creatorID)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = env.serve(http.MethodGet, "/api/v1/bookmarks/"+created.ID, "", creatorID)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBookmarkAPIValidationAndPermissions(t *testing.T) {
	env := newAPITestEnv(t)
	teamID, channelID := model.NewId(), model.NewId()
	actorID := model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	t.Run("invalid JSON", func(t *testing.T) {
		w := env.serve(http.MethodPost, "/api/v1/bookmarks", `{"title":`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("unknown field", func(t *testing.T) {
		w := env.serve(http.MethodPost, "/api/v1/bookmarks", `{"title":"x","bogus":true}`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("javascript URL", func(t *testing.T) {
		body := `{"channelId":"` + channelID + `","url":"javascript:alert(1)","title":"x"}`
		w := env.serve(http.MethodPost, "/api/v1/bookmarks", body, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("credentials in URL", func(t *testing.T) {
		body := `{"channelId":"` + channelID + `","url":"https://user:pass@example.com","title":"x"}`
		w := env.serve(http.MethodPost, "/api/v1/bookmarks", body, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("invalid ID", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/bookmarks/not-an-id", "", actorID)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("empty patch", func(t *testing.T) {
		w := env.serve(http.MethodPatch, "/api/v1/bookmarks/"+model.NewId(), `{}`, actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("team list forbidden", func(t *testing.T) {
		env.api.On("HasPermissionToTeam", actorID, teamID, model.PermissionViewTeam).Return(false).Maybe()
		w := env.serve(http.MethodGet, "/api/v1/bookmarks?scope=team&team_id="+teamID, "", actorID)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("invalid pagination", func(t *testing.T) {
		w := env.serve(http.MethodGet, "/api/v1/bookmarks?scope=my&page=abc", "", actorID)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAPISetsNoStoreAndContentType(t *testing.T) {
	env := newAPITestEnv(t)
	actorID := model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	w := env.serve(http.MethodGet, "/api/v1/bookmarks?scope=channel&channel_id="+channelID, "", actorID)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}
