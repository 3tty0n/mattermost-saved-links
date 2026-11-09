package kvstore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

func newTestStore() *KVStore {
	return NewStoreWithKV(NewMemoryKV(), WithClock(func() int64 { return 1000 }))
}

func testBookmark(id, teamID, channelID string) *bookmarkmodel.Bookmark {
	return &bookmarkmodel.Bookmark{
		ID:        id,
		TeamID:    teamID,
		ChannelID: channelID,
		URL:       "https://example.com/" + id,
		Title:     "Bookmark " + id,
		Tags:      []string{"docs"},
		CreatorID: "creator",
		CreatedAt: 1,
		UpdatedAt: 1,
		Version:   1,
	}
}

func TestBookmarkStoreIndexLifecycle(t *testing.T) {
	store := newTestStore()
	bookmark := testBookmark("bookmark-1", "team-a", "channel-a")
	require.NoError(t, store.CreateBookmark(bookmark))

	teamRefs, err := store.ListBookmarkRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 1)
	assert.Equal(t, "bookmark-1", teamRefs[0].ID)
	assert.Equal(t, "https://example.com/bookmark-1", teamRefs[0].URL)

	channelRefs, err := store.ListBookmarkRefsByChannel("channel-a")
	require.NoError(t, err)
	require.Len(t, channelRefs, 1)

	updated, err := store.UpdateBookmark("bookmark-1", func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error) {
		current.Title = "Renamed"
		current.Tags = []string{"work"}
		return current, nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated.Version)
	assert.Equal(t, int64(1000), updated.UpdatedAt)

	teamRefs, err = store.ListBookmarkRefsByTeam("team-a")
	require.NoError(t, err)
	require.Len(t, teamRefs, 1)
	assert.Equal(t, "Renamed", teamRefs[0].Title)
	assert.Equal(t, []string{"work"}, teamRefs[0].Tags)

	// Moving scope removes the old refs and adds the new ones.
	_, err = store.UpdateBookmark("bookmark-1", func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error) {
		current.TeamID = "team-b"
		current.ChannelID = ""
		return current, nil
	})
	require.NoError(t, err)

	oldTeam, err := store.ListBookmarkRefsByTeam("team-a")
	require.NoError(t, err)
	assert.Empty(t, oldTeam)
	oldChannel, err := store.ListBookmarkRefsByChannel("channel-a")
	require.NoError(t, err)
	assert.Empty(t, oldChannel)

	newTeam, err := store.ListBookmarkRefsByTeam("team-b")
	require.NoError(t, err)
	require.Len(t, newTeam, 1)

	// Description-only changes do not rewrite index records.
	before, err := store.ListBookmarkRefsByTeam("team-b")
	require.NoError(t, err)
	_, err = store.UpdateBookmark("bookmark-1", func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error) {
		current.Description = "details"
		return current, nil
	})
	require.NoError(t, err)
	after, err := store.ListBookmarkRefsByTeam("team-b")
	require.NoError(t, err)
	assert.Equal(t, before, after)

	require.NoError(t, store.DeleteBookmark("bookmark-1"))
	_, err = store.GetBookmark("bookmark-1")
	assert.ErrorIs(t, err, ErrNotFound)
	newTeam, err = store.ListBookmarkRefsByTeam("team-b")
	require.NoError(t, err)
	assert.Empty(t, newTeam)
}

func TestBookmarkStoreRejectsDuplicate(t *testing.T) {
	store := newTestStore()
	bookmark := testBookmark("bookmark-1", "team", "")
	require.NoError(t, store.CreateBookmark(bookmark))
	assert.ErrorIs(t, store.CreateBookmark(bookmark), ErrExists)
}

func TestBookmarkStoreUpdateAndDeleteNotFound(t *testing.T) {
	store := newTestStore()
	_, err := store.UpdateBookmark("missing", func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error) {
		return current, nil
	})
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, store.DeleteBookmark("missing"), ErrNotFound)
}

func TestBookmarkStoreConcurrentUpdates(t *testing.T) {
	store := newTestStore()
	bookmark := testBookmark("bookmark-1", "team", "channel")
	require.NoError(t, store.CreateBookmark(bookmark))

	const updates = 20
	var wg sync.WaitGroup
	errs := make(chan error, updates)
	for i := range updates {
		wg.Go(func() {
			_, err := store.UpdateBookmark("bookmark-1", func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error) {
				current.Title = fmt.Sprintf("Title %d", i)
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

	final, err := store.GetBookmark("bookmark-1")
	require.NoError(t, err)
	assert.Equal(t, int64(updates+1), final.Version)

	refs, err := store.ListBookmarkRefsByTeam("team")
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, final.Title, refs[0].Title)
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
