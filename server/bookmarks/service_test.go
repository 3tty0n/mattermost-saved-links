package bookmarks

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/3tty0n/mattermost-bookmarks/server/access"
	taskmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
	"github.com/3tty0n/mattermost-bookmarks/server/store/kvstore"
	"github.com/3tty0n/mattermost-bookmarks/server/testutil"
)

type fakePublisher struct {
	events    []string
	bookmarks []*taskmodel.Bookmark
}

func (p *fakePublisher) BookmarkChanged(action string, bookmark *taskmodel.Bookmark) {
	p.events = append(p.events, action)
	p.bookmarks = append(p.bookmarks, bookmark)
}

type testEnv struct {
	service   *Service
	api       *testutil.API
	store     kvstore.Store
	publisher *fakePublisher
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	api := testutil.NewAPI()
	t.Cleanup(func() { api.AssertExpectations(t) })
	client := pluginapi.NewClient(api, &plugintest.Driver{})
	store := kvstore.NewStoreWithKV(kvstore.NewMemoryKV())
	publisher := &fakePublisher{}
	return &testEnv{
		api:       api,
		store:     store,
		publisher: publisher,
		service:   NewService(client, store, access.NewChecker(client), publisher, WithClock(func() int64 { return 1000 })),
	}
}

func allowChannel(api *testutil.API, userID, channelID, teamID string) {
	api.On("GetChannel", channelID).Return(&model.Channel{Id: channelID, TeamId: teamID, Type: model.ChannelTypeOpen}, nil).Maybe()
	api.On("HasPermissionToChannel", userID, channelID, model.PermissionReadChannel).Return(true).Maybe()
}

func TestCreateBookmark(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	bookmark, err := env.service.Create(nil, "/bookmarks", actorID, taskmodel.CreateBookmarkInput{
		ChannelID: channelID,
		URL:       "https://example.com/docs",
		Title:     " Docs ",
		Tags:      []string{"Work", "work"},
	})
	require.NoError(t, err)
	assert.Equal(t, teamID, bookmark.TeamID)
	assert.Equal(t, channelID, bookmark.ChannelID)
	assert.Equal(t, "https://example.com/docs", bookmark.URL)
	assert.Equal(t, "Docs", bookmark.Title)
	assert.Equal(t, []string{"Work"}, bookmark.Tags)
	assert.Equal(t, 1, env.api.AuditSuccesses())
	assert.Equal(t, []string{"created"}, env.publisher.events)

	refs, err := env.store.ListBookmarkRefsByChannel(channelID)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://example.com/docs", refs[0].URL)
}

func TestCreateBookmarkFromPost(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID, postID := model.NewId(), model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	env.api.On("GetPost", postID).Return(&model.Post{Id: postID, ChannelId: channelID}, nil).Maybe()

	bookmark, err := env.service.Create(nil, "/bookmarks", actorID, taskmodel.CreateBookmarkInput{
		URL:          "https://example.com",
		Title:        "Post link",
		SourcePostID: postID,
	})
	require.NoError(t, err)
	assert.Equal(t, channelID, bookmark.ChannelID)
	assert.Equal(t, postID, bookmark.SourcePostID)
	refs, err := env.store.ListBookmarkRefsByChannel(channelID)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, postID, refs[0].SourcePostID)

	_, err = env.service.Create(nil, "/bookmarks", actorID, taskmodel.CreateBookmarkInput{
		ChannelID:    model.NewId(),
		URL:          "https://example.com",
		Title:        "Wrong",
		SourcePostID: postID,
	})
	assert.ErrorIs(t, err, taskmodel.ErrInvalid)
}

func TestCreateBookmarkValidatesInput(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	for _, input := range []taskmodel.CreateBookmarkInput{
		{ChannelID: channelID, URL: "javascript:alert(1)", Title: "x"},
		{ChannelID: channelID, URL: "https://example.com", Title: ""},
		{ChannelID: channelID, URL: "https://example.com", Title: "x", Tags: []string{string(make([]rune, taskmodel.MaxTagRunes+1))}},
		{ChannelID: "bad", URL: "https://example.com", Title: "x"},
	} {
		_, err := env.service.Create(nil, "/bookmarks", actorID, input)
		assert.ErrorIs(t, err, taskmodel.ErrInvalid)
	}
}

func TestListBookmarksKeepsPrivateChannelsPrivate(t *testing.T) {
	env := newTestEnv(t)
	actorID, teamID := model.NewId(), model.NewId()
	openChannel, privateChannel := model.NewId(), model.NewId()

	require.NoError(t, env.store.CreateBookmark(&taskmodel.Bookmark{
		ID: model.NewId(), TeamID: teamID, ChannelID: openChannel, URL: "https://a.example", Title: "Open",
		CreatorID: "creator", Tags: []string{"docs"}, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}))
	require.NoError(t, env.store.CreateBookmark(&taskmodel.Bookmark{
		ID: model.NewId(), TeamID: teamID, ChannelID: privateChannel, URL: "https://b.example", Title: "Private",
		CreatorID: "creator", Tags: []string{"secret"}, CreatedAt: 2, UpdatedAt: 2, Version: 1,
	}))

	env.api.On("HasPermissionToTeam", actorID, teamID, model.PermissionViewTeam).Return(true).Maybe()
	env.api.On("HasPermissionToChannel", actorID, openChannel, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionToChannel", actorID, privateChannel, model.PermissionReadChannel).Return(false).Maybe()

	result, err := env.service.List(actorID, taskmodel.BookmarkQuery{Scope: taskmodel.BookmarkScopeTeam, TeamID: teamID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "Open", result.Items[0].Title)
}

func TestSearchAndFilterBookmarks(t *testing.T) {
	env := newTestEnv(t)
	actorID, channelID, teamID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	for _, bookmark := range []*taskmodel.Bookmark{
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/runbook", Title: "Runbook", CreatorID: actorID, Tags: []string{"ops"}, CreatedAt: 3, UpdatedAt: 3, Version: 1},
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/design", Title: "Design", CreatorID: actorID, Tags: []string{"design"}, CreatedAt: 2, UpdatedAt: 2, Version: 1},
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/ops", Title: "On-call", CreatorID: actorID, Tags: []string{"ops"}, CreatedAt: 1, UpdatedAt: 1, Version: 1},
	} {
		require.NoError(t, env.store.CreateBookmark(bookmark))
	}

	result, err := env.service.List(actorID, taskmodel.BookmarkQuery{
		Scope:     taskmodel.BookmarkScopeChannel,
		ChannelID: channelID,
		Tag:       "ops",
		PerPage:   10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
	assert.Equal(t, []string{"Runbook", "On-call"}, []string{result.Items[0].Title, result.Items[1].Title})

	result, err = env.service.List(actorID, taskmodel.BookmarkQuery{
		Scope:     taskmodel.BookmarkScopeChannel,
		ChannelID: channelID,
		Search:    "design",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Design", result.Items[0].Title)

	result, err = env.service.List(actorID, taskmodel.BookmarkQuery{Scope: taskmodel.BookmarkScopeChannel, ChannelID: channelID, Page: 1, PerPage: 2})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total-2)
	assert.Len(t, result.Items, 1)
	assert.False(t, result.HasMore)
}

func TestUpdateBookmarkPermissions(t *testing.T) {
	env := newTestEnv(t)
	creatorID, strangerID, adminID := model.NewId(), model.NewId(), model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	bookmark := &taskmodel.Bookmark{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com", Title: "Original",
		CreatorID: creatorID, Tags: []string{"old"}, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	require.NoError(t, env.store.CreateBookmark(bookmark))

	env.api.On("HasPermissionToChannel", creatorID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", creatorID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", strangerID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", adminID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", adminID, model.PermissionManageSystem).Return(true).Maybe()

	title := "Renamed"
	_, err := env.service.Update(nil, "/bookmarks", strangerID, bookmark.ID, taskmodel.BookmarkPatch{Title: &title})
	assert.ErrorIs(t, err, taskmodel.ErrForbidden)

	updated, err := env.service.Update(nil, "/bookmarks", creatorID, bookmark.ID, taskmodel.BookmarkPatch{Title: &title})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Title)
	assert.Equal(t, []string{"updated"}, env.publisher.events)

	adminTitle := "Admin"
	updated, err = env.service.Update(nil, "/bookmarks", adminID, bookmark.ID, taskmodel.BookmarkPatch{Title: &adminTitle})
	require.NoError(t, err)
	assert.Equal(t, "Admin", updated.Title)
}

func TestDeleteBookmarkPermissions(t *testing.T) {
	env := newTestEnv(t)
	creatorID, strangerID := model.NewId(), model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	bookmark := &taskmodel.Bookmark{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com", Title: "Delete",
		CreatorID: creatorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	require.NoError(t, env.store.CreateBookmark(bookmark))

	env.api.On("HasPermissionToChannel", creatorID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", creatorID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(false).Maybe()

	assert.ErrorIs(t, env.service.Delete(nil, "/bookmarks", strangerID, bookmark.ID), taskmodel.ErrNotFound)
	require.NoError(t, env.service.Delete(nil, "/bookmarks", creatorID, bookmark.ID))
	_, err := env.store.GetBookmark(bookmark.ID)
	assert.ErrorIs(t, err, kvstore.ErrNotFound)
}
