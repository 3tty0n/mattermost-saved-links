package savedlinks

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/3tty0n/mattermost-saved-links/server/access"
	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
	"github.com/3tty0n/mattermost-saved-links/server/store/kvstore"
	"github.com/3tty0n/mattermost-saved-links/server/testutil"
)

type fakePublisher struct {
	events       []string
	savedLinks   []*savedlinkmodel.SavedLink
	folderEvents []string
	folders      []*savedlinkmodel.Folder
}

func (p *fakePublisher) SavedLinkChanged(action string, savedLink *savedlinkmodel.SavedLink) {
	p.events = append(p.events, action)
	p.savedLinks = append(p.savedLinks, savedLink)
}

func (p *fakePublisher) FolderChanged(action string, folder *savedlinkmodel.Folder) {
	p.folderEvents = append(p.folderEvents, action)
	p.folders = append(p.folders, folder)
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

func allowTeam(api *testutil.API, userID, teamID string) {
	api.On("GetTeam", teamID).Return(&model.Team{Id: teamID}, nil).Maybe()
	api.On("HasPermissionToTeam", userID, teamID, model.PermissionViewTeam).Return(true).Maybe()
}

func TestCreateSavedLink(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	savedLink, err := env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelID,
		URL:       "https://example.com/docs",
		Name:      " Docs ",
		Tags:      []string{"Work", "work"},
	})
	require.NoError(t, err)
	assert.Equal(t, teamID, savedLink.TeamID)
	assert.Equal(t, channelID, savedLink.ChannelID)
	assert.Equal(t, "https://example.com/docs", savedLink.URL)
	assert.Equal(t, "Docs", savedLink.Name)
	assert.Equal(t, []string{"Work"}, savedLink.Tags)
	assert.Empty(t, savedLink.FolderID)
	assert.Equal(t, int64(1000), savedLink.Position)
	assert.Equal(t, 1, env.api.AuditSuccesses())
	assert.Equal(t, []string{"created"}, env.publisher.events)

	refs, err := env.store.ListSavedLinkRefsByChannel(channelID)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://example.com/docs", refs[0].URL)
}

func TestCreateSavedLinkWithURLOnly(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	savedLink, err := env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelID,
		URL:       "https://www.example.com/getting-started",
	})
	require.NoError(t, err)
	assert.Equal(t, "getting started", savedLink.Name)
}

func TestCreateSavedLinkFromPost(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID, postID := model.NewId(), model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	env.api.On("GetPost", postID).Return(&model.Post{Id: postID, ChannelId: channelID}, nil).Maybe()

	savedLink, err := env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		URL:          "https://example.com",
		Name:         "Post link",
		SourcePostID: postID,
	})
	require.NoError(t, err)
	assert.Equal(t, channelID, savedLink.ChannelID)
	assert.Equal(t, postID, savedLink.SourcePostID)
	refs, err := env.store.ListSavedLinkRefsByChannel(channelID)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, postID, refs[0].SourcePostID)

	_, err = env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID:    model.NewId(),
		URL:          "https://example.com",
		Name:         "Wrong",
		SourcePostID: postID,
	})
	assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)
}

func TestCreateSavedLinkValidatesInput(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	for _, input := range []savedlinkmodel.CreateSavedLinkInput{
		{ChannelID: channelID, URL: "javascript:alert(1)", Name: "x"},
		{ChannelID: channelID, URL: "https://example.com", Name: string(make([]rune, savedlinkmodel.MaxSavedLinkNameRunes+1))},
		{ChannelID: channelID, URL: "https://example.com", Name: "x", Tags: []string{string(make([]rune, savedlinkmodel.MaxTagRunes+1))}},
		{ChannelID: "bad", URL: "https://example.com", Name: "x"},
		{ChannelID: channelID, URL: "https://example.com", Name: "x", FolderID: "bad"},
	} {
		_, err := env.service.Create(nil, "/saved-links", actorID, input)
		assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)
	}
}

func TestListSavedLinksKeepsPrivateChannelsPrivate(t *testing.T) {
	env := newTestEnv(t)
	actorID, teamID := model.NewId(), model.NewId()
	openChannel, privateChannel := model.NewId(), model.NewId()

	require.NoError(t, env.store.CreateSavedLink(&savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: openChannel, URL: "https://a.example", Name: "Open",
		CreatorID: "creator", Tags: []string{"docs"}, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}))
	require.NoError(t, env.store.CreateSavedLink(&savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: privateChannel, URL: "https://b.example", Name: "Private",
		CreatorID: "creator", Tags: []string{"secret"}, CreatedAt: 2, UpdatedAt: 2, Version: 1,
	}))

	env.api.On("HasPermissionToTeam", actorID, teamID, model.PermissionViewTeam).Return(true).Maybe()
	env.api.On("HasPermissionToChannel", actorID, openChannel, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionToChannel", actorID, privateChannel, model.PermissionReadChannel).Return(false).Maybe()

	result, err := env.service.List(actorID, savedlinkmodel.SavedLinkQuery{Scope: savedlinkmodel.SavedLinkScopeTeam, TeamID: teamID})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "Open", result.Items[0].Name)
}

func TestSearchAndFilterSavedLinks(t *testing.T) {
	env := newTestEnv(t)
	actorID, channelID, teamID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	for _, savedLink := range []*savedlinkmodel.SavedLink{
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/runbook", Name: "Runbook", CreatorID: actorID, Tags: []string{"ops"}, CreatedAt: 3, UpdatedAt: 3, Version: 1},
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/design", Name: "Design", CreatorID: actorID, Tags: []string{"design"}, CreatedAt: 2, UpdatedAt: 2, Version: 1},
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/ops", Name: "On-call", CreatorID: actorID, Tags: []string{"ops"}, CreatedAt: 1, UpdatedAt: 1, Version: 1},
	} {
		require.NoError(t, env.store.CreateSavedLink(savedLink))
	}

	result, err := env.service.List(actorID, savedlinkmodel.SavedLinkQuery{
		Scope:     savedlinkmodel.SavedLinkScopeChannel,
		ChannelID: channelID,
		Tag:       "ops",
		PerPage:   10,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
	assert.Equal(t, []string{"Runbook", "On-call"}, []string{result.Items[0].Name, result.Items[1].Name})

	result, err = env.service.List(actorID, savedlinkmodel.SavedLinkQuery{
		Scope:     savedlinkmodel.SavedLinkScopeChannel,
		ChannelID: channelID,
		Search:    "design",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Design", result.Items[0].Name)

	result, err = env.service.List(actorID, savedlinkmodel.SavedLinkQuery{Scope: savedlinkmodel.SavedLinkScopeChannel, ChannelID: channelID, Page: 1, PerPage: 2})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total-2)
	assert.Len(t, result.Items, 1)
	assert.False(t, result.HasMore)
}

func TestListSavedLinksByFolder(t *testing.T) {
	env := newTestEnv(t)
	actorID, channelID, teamID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	folder, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelID, Name: "Work"})
	require.NoError(t, err)

	for _, savedLink := range []*savedlinkmodel.SavedLink{
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, FolderID: folder.ID, URL: "https://example.com/one", Name: "One", CreatorID: actorID, CreatedAt: 2, UpdatedAt: 2, Version: 1},
		{ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com/two", Name: "Two", CreatorID: actorID, CreatedAt: 1, UpdatedAt: 1, Version: 1},
	} {
		require.NoError(t, env.store.CreateSavedLink(savedLink))
	}

	result, err := env.service.List(actorID, savedlinkmodel.SavedLinkQuery{
		Scope: savedlinkmodel.SavedLinkScopeChannel, ChannelID: channelID, FolderID: folder.ID,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "One", result.Items[0].Name)

	result, err = env.service.List(actorID, savedlinkmodel.SavedLinkQuery{
		Scope: savedlinkmodel.SavedLinkScopeChannel, ChannelID: channelID, FolderID: savedlinkmodel.SavedLinkFolderNone,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "Two", result.Items[0].Name)
}

func TestReorderSavedLinks(t *testing.T) {
	env := newTestEnv(t)
	actorID, channelID, teamID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	first, second, third := model.NewId(), model.NewId(), model.NewId()
	for _, savedLink := range []*savedlinkmodel.SavedLink{
		{ID: first, TeamID: teamID, ChannelID: channelID, URL: "https://example.com/1", Name: "First", CreatorID: actorID, Position: 1000, CreatedAt: 1, UpdatedAt: 1, Version: 1},
		{ID: second, TeamID: teamID, ChannelID: channelID, URL: "https://example.com/2", Name: "Second", CreatorID: actorID, Position: 2000, CreatedAt: 2, UpdatedAt: 2, Version: 1},
		{ID: third, TeamID: teamID, ChannelID: channelID, URL: "https://example.com/3", Name: "Third", CreatorID: actorID, Position: 3000, CreatedAt: 3, UpdatedAt: 3, Version: 1},
	} {
		require.NoError(t, env.store.CreateSavedLink(savedLink))
	}

	require.NoError(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{third, first, second}))

	result, err := env.service.List(actorID, savedlinkmodel.SavedLinkQuery{
		Scope: savedlinkmodel.SavedLinkScopeChannel, ChannelID: channelID,
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	assert.Equal(t, []string{"Third", "First", "Second"}, []string{
		result.Items[0].Name, result.Items[1].Name, result.Items[2].Name,
	})
	assert.Equal(t, []string{"reordered"}, env.publisher.events)

	for expected, id := range []string{third, first, second} {
		savedLink, getErr := env.store.GetSavedLink(id)
		require.NoError(t, getErr)
		assert.Equal(t, int64(expected+1)*1000, savedLink.Position)
	}
}

func TestReorderSavedLinksRejectsInvalidInput(t *testing.T) {
	env := newTestEnv(t)
	actorID, teamID := model.NewId(), model.NewId()
	channelID, otherChannelID := model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	env.api.On("HasPermissionToChannel", actorID, otherChannelID, model.PermissionReadChannel).Return(false).Maybe()

	inFolderA := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, FolderID: "folder-a",
		URL: "https://example.com/a", Name: "A", CreatorID: actorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	inFolderB := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, FolderID: "folder-b",
		URL: "https://example.com/b", Name: "B", CreatorID: actorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	inOtherChannel := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: otherChannelID,
		URL: "https://example.com/c", Name: "C", CreatorID: actorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	for _, savedLink := range []*savedlinkmodel.SavedLink{inFolderA, inFolderB, inOtherChannel} {
		require.NoError(t, env.store.CreateSavedLink(savedLink))
	}

	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, nil), savedlinkmodel.ErrInvalid)
	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{"bad"}), savedlinkmodel.ErrInvalid)
	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{inFolderA.ID, inFolderA.ID}), savedlinkmodel.ErrInvalid)
	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{inFolderA.ID, inFolderB.ID}), savedlinkmodel.ErrInvalid)
	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{model.NewId()}), savedlinkmodel.ErrNotFound)
	assert.ErrorIs(t, env.service.Reorder(nil, "/saved-links/reorder", actorID, []string{inOtherChannel.ID}), savedlinkmodel.ErrNotFound)
}

func TestUpdateSavedLinkMoveAppendsPosition(t *testing.T) {
	env := newTestEnv(t)
	actorID, teamID, channelID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	folder, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelID, Name: "Work"})
	require.NoError(t, err)
	savedLink := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, Position: 1,
		URL: "https://example.com", Name: "One", CreatorID: actorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	require.NoError(t, env.store.CreateSavedLink(savedLink))

	updated, err := env.service.Update(nil, "/saved-links", actorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{FolderID: &folder.ID})
	require.NoError(t, err)
	assert.Equal(t, folder.ID, updated.FolderID)
	assert.Equal(t, int64(1000), updated.Position)
}

func TestUpdateSavedLinkPermissions(t *testing.T) {
	env := newTestEnv(t)
	creatorID, strangerID, adminID := model.NewId(), model.NewId(), model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	savedLink := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com", Name: "Original",
		CreatorID: creatorID, Tags: []string{"old"}, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	require.NoError(t, env.store.CreateSavedLink(savedLink))

	env.api.On("HasPermissionToChannel", creatorID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", creatorID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", strangerID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", adminID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", adminID, model.PermissionManageSystem).Return(true).Maybe()

	name := "Renamed"
	_, err := env.service.Update(nil, "/saved-links", strangerID, savedLink.ID, savedlinkmodel.SavedLinkPatch{Name: &name})
	assert.ErrorIs(t, err, savedlinkmodel.ErrForbidden)

	updated, err := env.service.Update(nil, "/saved-links", creatorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{Name: &name})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, []string{"updated"}, env.publisher.events)

	adminName := "Admin"
	updated, err = env.service.Update(nil, "/saved-links", adminID, savedLink.ID, savedlinkmodel.SavedLinkPatch{Name: &adminName})
	require.NoError(t, err)
	assert.Equal(t, "Admin", updated.Name)
}

func TestUpdateSavedLinkURLOnlyNameAndFolder(t *testing.T) {
	env := newTestEnv(t)
	creatorID, teamID, channelID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, creatorID, channelID, teamID)

	folder, err := env.service.CreateFolder(nil, "/folders", creatorID, savedlinkmodel.CreateFolderInput{ChannelID: channelID, Name: "Work"})
	require.NoError(t, err)
	savedLink, err := env.service.Create(nil, "/saved-links", creatorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelID, URL: "https://example.com/one", Name: "One",
	})
	require.NoError(t, err)

	empty := ""
	updated, err := env.service.Update(nil, "/saved-links", creatorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{
		Name:     &empty,
		URL:      ptr("https://example.com/two"),
		FolderID: &folder.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "two", updated.Name)
	assert.Equal(t, folder.ID, updated.FolderID)

	cleared := ""
	updated, err = env.service.Update(nil, "/saved-links", creatorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{FolderID: &cleared})
	require.NoError(t, err)
	assert.Empty(t, updated.FolderID)
}

func TestUpdateSavedLinkFolderMustMatchScope(t *testing.T) {
	env := newTestEnv(t)
	actorID, teamID := model.NewId(), model.NewId()
	channelA, channelB := model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelA, teamID)
	allowChannel(env.api, actorID, channelB, teamID)

	folderA, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelA, Name: "A"})
	require.NoError(t, err)
	folderB, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelB, Name: "B"})
	require.NoError(t, err)

	savedLink, err := env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelA, URL: "https://example.com", Name: "One", FolderID: folderA.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, folderA.ID, savedLink.FolderID)

	// An explicit folder from another scope is rejected.
	_, err = env.service.Update(nil, "/saved-links", actorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{FolderID: &folderB.ID})
	assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)

	// Moving the saved link to another channel without an explicit folder clears the folder.
	updated, err := env.service.Update(nil, "/saved-links", actorID, savedLink.ID, savedlinkmodel.SavedLinkPatch{ChannelID: &channelB})
	require.NoError(t, err)
	assert.Equal(t, channelB, updated.ChannelID)
	assert.Empty(t, updated.FolderID)
}

func TestDeleteSavedLinkPermissions(t *testing.T) {
	env := newTestEnv(t)
	creatorID, strangerID := model.NewId(), model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	savedLink := &savedlinkmodel.SavedLink{
		ID: model.NewId(), TeamID: teamID, ChannelID: channelID, URL: "https://example.com", Name: "Delete",
		CreatorID: creatorID, CreatedAt: 1, UpdatedAt: 1, Version: 1,
	}
	require.NoError(t, env.store.CreateSavedLink(savedLink))

	env.api.On("HasPermissionToChannel", creatorID, channelID, model.PermissionReadChannel).Return(true).Maybe()
	env.api.On("HasPermissionTo", creatorID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionToChannel", strangerID, channelID, model.PermissionReadChannel).Return(false).Maybe()

	assert.ErrorIs(t, env.service.Delete(nil, "/saved-links", strangerID, savedLink.ID), savedlinkmodel.ErrNotFound)
	require.NoError(t, env.service.Delete(nil, "/saved-links", creatorID, savedLink.ID))
	_, err := env.store.GetSavedLink(savedLink.ID)
	assert.ErrorIs(t, err, kvstore.ErrNotFound)
}

func TestFolderLifecycle(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)

	folder, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{
		ChannelID: channelID,
		Name:      " Work ",
	})
	require.NoError(t, err)
	assert.Equal(t, teamID, folder.TeamID)
	assert.Equal(t, channelID, folder.ChannelID)
	assert.Equal(t, "Work", folder.Name)
	assert.Equal(t, []string{"created"}, env.publisher.folderEvents)

	listed, err := env.service.ListFolders(actorID, "", channelID)
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, folder.ID, listed.Items[0].ID)
	assert.Equal(t, 0, listed.Items[0].SavedLinkCount)

	savedLink, err := env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelID,
		FolderID:  folder.ID,
		URL:       "https://example.com/docs",
	})
	require.NoError(t, err)
	assert.Equal(t, folder.ID, savedLink.FolderID)

	listed, err = env.service.ListFolders(actorID, "", channelID)
	require.NoError(t, err)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, 1, listed.Items[0].SavedLinkCount)

	renamed := "Reference"
	updated, err := env.service.UpdateFolder(nil, "/folders", actorID, folder.ID, savedlinkmodel.FolderPatch{Name: &renamed})
	require.NoError(t, err)
	assert.Equal(t, "Reference", updated.Name)
	assert.Equal(t, []string{"created", "updated"}, env.publisher.folderEvents)

	retrieved, err := env.service.GetFolder(actorID, folder.ID)
	require.NoError(t, err)
	assert.Equal(t, "Reference", retrieved.Name)

	require.NoError(t, env.service.DeleteFolder(nil, "/folders", actorID, folder.ID))
	assert.Equal(t, []string{"created", "updated", "deleted"}, env.publisher.folderEvents)
	assert.Equal(t, []string{"created", "updated"}, env.publisher.events)

	_, err = env.store.GetFolder(folder.ID)
	assert.ErrorIs(t, err, kvstore.ErrNotFound)
	restored, err := env.store.GetSavedLink(savedLink.ID)
	require.NoError(t, err)
	assert.Empty(t, restored.FolderID)

	listed, err = env.service.ListFolders(actorID, "", channelID)
	require.NoError(t, err)
	assert.Empty(t, listed.Items)
}

func TestFolderScopeAndNameValidation(t *testing.T) {
	env := newTestEnv(t)
	teamID, channelID, actorID := model.NewId(), model.NewId(), model.NewId()
	allowChannel(env.api, actorID, channelID, teamID)
	allowTeam(env.api, actorID, teamID)

	_, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelID, Name: "Work"})
	require.NoError(t, err)

	_, err = env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{ChannelID: channelID, Name: " work "})
	assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)

	_, err = env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{Name: "No scope"})
	assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)

	// Team and channel folders are listed separately.
	teamFolder, err := env.service.CreateFolder(nil, "/folders", actorID, savedlinkmodel.CreateFolderInput{TeamID: teamID, Name: "Team folder"})
	require.NoError(t, err)
	assert.Empty(t, teamFolder.ChannelID)

	channelFolders, err := env.service.ListFolders(actorID, "", channelID)
	require.NoError(t, err)
	require.Len(t, channelFolders.Items, 1)
	assert.Equal(t, "Work", channelFolders.Items[0].Name)

	teamFolders, err := env.service.ListFolders(actorID, teamID, "")
	require.NoError(t, err)
	require.Len(t, teamFolders.Items, 1)
	assert.Equal(t, "Team folder", teamFolders.Items[0].Name)

	// A saved link can not reference a folder from another scope.
	_, err = env.service.Create(nil, "/saved-links", actorID, savedlinkmodel.CreateSavedLinkInput{
		ChannelID: channelID,
		FolderID:  teamFolder.ID,
		URL:       "https://example.com",
	})
	assert.ErrorIs(t, err, savedlinkmodel.ErrInvalid)
}

func TestFolderPermissions(t *testing.T) {
	env := newTestEnv(t)
	creatorID, strangerID, adminID := model.NewId(), model.NewId(), model.NewId()
	teamID, channelID := model.NewId(), model.NewId()
	for _, userID := range []string{creatorID, strangerID, adminID} {
		allowChannel(env.api, userID, channelID, teamID)
	}
	env.api.On("HasPermissionTo", creatorID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionTo", strangerID, model.PermissionManageSystem).Return(false).Maybe()
	env.api.On("HasPermissionTo", adminID, model.PermissionManageSystem).Return(true).Maybe()

	folder, err := env.service.CreateFolder(nil, "/folders", creatorID, savedlinkmodel.CreateFolderInput{
		ChannelID: channelID,
		Name:      "Work",
	})
	require.NoError(t, err)

	name := "Renamed"
	_, err = env.service.UpdateFolder(nil, "/folders", strangerID, folder.ID, savedlinkmodel.FolderPatch{Name: &name})
	assert.ErrorIs(t, err, savedlinkmodel.ErrForbidden)

	updated, err := env.service.UpdateFolder(nil, "/folders", adminID, folder.ID, savedlinkmodel.FolderPatch{Name: &name})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Name)

	assert.ErrorIs(t, env.service.DeleteFolder(nil, "/folders", strangerID, folder.ID), savedlinkmodel.ErrForbidden)
	require.NoError(t, env.service.DeleteFolder(nil, "/folders", creatorID, folder.ID))
}

func ptr[T any](value T) *T {
	return &value
}
