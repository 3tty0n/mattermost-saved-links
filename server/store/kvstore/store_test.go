package kvstore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

func newTestStore() *KVStore {
	return NewStoreWithKV(NewMemoryKV(), WithClock(func() int64 { return 1000 }))
}

func testSavedLink(id, teamID, channelID string) *savedlinkmodel.SavedLink {
	return &savedlinkmodel.SavedLink{
		ID:        id,
		TeamID:    teamID,
		ChannelID: channelID,
		URL:       "https://example.com/" + id,
		Name:      "Saved link " + id,
		Tags:      []string{"docs"},
		CreatorID: "creator",
		CreatedAt: 1,
		UpdatedAt: 1,
		Version:   1,
	}
}

func testFolder(id, teamID, channelID, name string) *savedlinkmodel.Folder {
	return &savedlinkmodel.Folder{
		ID:        id,
		TeamID:    teamID,
		ChannelID: channelID,
		Name:      name,
		CreatorID: "creator",
		CreatedAt: 1,
		UpdatedAt: 1,
		Version:   1,
	}
}

func TestSavedLinkStoreIndexLifecycle(t *testing.T) {
	store := newTestStore()
	savedLink := testSavedLink("link-1", "team-a", "channel-a")
	require.NoError(t, store.CreateSavedLink(savedLink))

	teamRefs, err := store.ListSavedLinkRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 1)
	assert.Equal(t, "link-1", teamRefs[0].ID)
	assert.Equal(t, "https://example.com/link-1", teamRefs[0].URL)

	channelRefs, err := store.ListSavedLinkRefsByChannel("channel-a")
	require.NoError(t, err)
	require.Len(t, channelRefs, 1)

	updated, err := store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		current.Name = "Renamed"
		current.Tags = []string{"work"}
		return current, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Version)
	assert.Equal(t, int64(1000), updated.UpdatedAt)

	teamRefs, err = store.ListSavedLinkRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 1)
	assert.Equal(t, "Renamed", teamRefs[0].Name)
	assert.Equal(t, []string{"work"}, teamRefs[0].Tags)

	// Moving scope removes the old refs and adds the new ones.
	_, err = store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		current.TeamID = "team-b"
		current.ChannelID = ""
		return current, nil
	})
	require.NoError(t, err)

	oldTeam, err := store.ListSavedLinkRefsByTeam("team-a")
	require.NoError(t, err)
	assert.Empty(t, oldTeam)
	oldChannel, err := store.ListSavedLinkRefsByChannel("channel-a")
	require.NoError(t, err)
	assert.Empty(t, oldChannel)

	newTeam, err := store.ListSavedLinkRefsByTeam("team-b")
	require.NoError(t, err)
	require.Len(t, newTeam, 1)

	// Updates that do not change indexed fields leave index records untouched.
	before, err := store.ListSavedLinkRefsByTeam("team-b")
	require.NoError(t, err)
	_, err = store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		current.Name = "Renamed"
		return current, nil
	})
	require.NoError(t, err)
	after, err := store.ListSavedLinkRefsByTeam("team-b")
	require.NoError(t, err)
	assert.Equal(t, before, after)

	require.NoError(t, store.DeleteSavedLink("link-1"))
	_, err = store.GetSavedLink("link-1")
	assert.ErrorIs(t, err, ErrNotFound)
	newTeam, err = store.ListSavedLinkRefsByTeam("team-b")
	require.NoError(t, err)
	assert.Empty(t, newTeam)
}

func TestSavedLinkStoreTracksFolder(t *testing.T) {
	store := newTestStore()
	savedLink := testSavedLink("link-1", "team", "channel")
	require.NoError(t, store.CreateSavedLink(savedLink))

	_, err := store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		current.FolderID = "folder-1"
		return current, nil
	})
	require.NoError(t, err)

	refs, err := store.ListSavedLinkRefsByChannel("channel")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "folder-1", refs[0].FolderID)

	_, err = store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		current.Position = 3000
		return current, nil
	})
	require.NoError(t, err)

	refs, err = store.ListSavedLinkRefsByChannel("channel")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, int64(3000), refs[0].Position)
}

func TestFolderStoreIndexLifecycle(t *testing.T) {
	store := newTestStore()
	channelFolder := testFolder("folder-channel", "team-a", "channel-a", "Channel folder")
	teamFolder := testFolder("folder-team", "team-a", "", "Team folder")
	require.NoError(t, store.CreateFolder(channelFolder))
	require.NoError(t, store.CreateFolder(teamFolder))

	teamRefs, err := store.ListFolderRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 2)

	channelRefs, err := store.ListFolderRefsByChannel("channel-a")
	require.NoError(t, err)
	require.Len(t, channelRefs, 1)
	assert.Equal(t, "Channel folder", channelRefs[0].Name)

	updated, err := store.UpdateFolder("folder-channel", func(current *savedlinkmodel.Folder) (*savedlinkmodel.Folder, error) {
		current.Name = "Renamed"
		return current, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Version)
	assert.Equal(t, int64(1000), updated.UpdatedAt)
	assert.Equal(t, "team-a", updated.TeamID)
	assert.Equal(t, "channel-a", updated.ChannelID)

	channelRefs, err = store.ListFolderRefsByChannel("channel-a")
	require.NoError(t, err)
	require.Len(t, channelRefs, 1)
	assert.Equal(t, "Renamed", channelRefs[0].Name)

	require.NoError(t, store.DeleteFolder("folder-channel"))
	_, err = store.GetFolder("folder-channel")
	assert.ErrorIs(t, err, ErrNotFound)
	channelRefs, err = store.ListFolderRefsByChannel("channel-a")
	require.NoError(t, err)
	assert.Empty(t, channelRefs)

	teamRefs, err = store.ListFolderRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 1)
	assert.Equal(t, "folder-team", teamRefs[0].ID)
}

func TestSavedLinkStoreRejectsDuplicate(t *testing.T) {
	store := newTestStore()
	savedLink := testSavedLink("link-1", "team", "")
	require.NoError(t, store.CreateSavedLink(savedLink))
	assert.ErrorIs(t, store.CreateSavedLink(savedLink), ErrExists)
}

func TestFolderStoreRejectsDuplicate(t *testing.T) {
	store := newTestStore()
	folder := testFolder("folder-1", "team", "", "Docs")
	require.NoError(t, store.CreateFolder(folder))
	assert.ErrorIs(t, store.CreateFolder(folder), ErrExists)
}

func TestSavedLinkStoreUpdateAndDeleteNotFound(t *testing.T) {
	store := newTestStore()
	_, err := store.UpdateSavedLink("missing", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
		return current, nil
	})
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, store.DeleteSavedLink("missing"), ErrNotFound)

	_, err = store.UpdateFolder("missing", func(current *savedlinkmodel.Folder) (*savedlinkmodel.Folder, error) {
		return current, nil
	})
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, store.DeleteFolder("missing"), ErrNotFound)
}

func TestSavedLinkStoreConcurrentUpdates(t *testing.T) {
	store := newTestStore()
	savedLink := testSavedLink("link-1", "team", "channel")
	require.NoError(t, store.CreateSavedLink(savedLink))

	const updates = 20
	var wg sync.WaitGroup
	errs := make(chan error, updates)
	for i := range updates {
		wg.Go(func() {
			_, err := store.UpdateSavedLink("link-1", func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error) {
				current.Name = fmt.Sprintf("Name %d", i)
				return current, nil
			})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	final, err := store.GetSavedLink("link-1")
	require.NoError(t, err)
	assert.Equal(t, int64(updates+1), final.Version)

	refs, err := store.ListSavedLinkRefsByTeam("team")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, final.Name, refs[0].Name)
}

func TestMemoryKVCompareAndSet(t *testing.T) {
	kv := NewMemoryKV()
	saved, err := kv.CompareAndSet("key", nil, map[string]string{"a": "b"})
	require.NoError(t, err)
	assert.True(t, saved)

	saved, err = kv.CompareAndSet("key", []byte(`{"a":"c"}`), map[string]string{"a": "d"})
	require.NoError(t, err)
	assert.False(t, saved)

	var value map[string]string
	require.NoError(t, kv.Get("key", &value))
	assert.Equal(t, map[string]string{"a": "b"}, value)
}

func TestMemoryKVUpdateNoChange(t *testing.T) {
	kv := NewMemoryKV()
	require.NoError(t, kv.Update("key", func([]byte) (any, error) {
		return nil, ErrNoChange
	}))
	var raw []byte
	require.NoError(t, kv.Get("key", &raw))
	assert.Empty(t, raw)
}
