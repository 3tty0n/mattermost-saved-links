// Package kvstore persists bookmarks in the Mattermost plugin KV store and maintains compact
// per-scope indexes for list views.
package kvstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

const (
	// Four shards bound each index value and spread compare-and-swap contention.
	indexShards = 4
	lockStripes = 64
	casRetries  = 5
)

// Store is the persistence interface used by the bookmark service.
type Store interface {
	GetBookmark(id string) (*bookmarkmodel.Bookmark, error)
	CreateBookmark(bookmark *bookmarkmodel.Bookmark) error
	UpdateBookmark(id string, mutate func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error)) (*bookmarkmodel.Bookmark, error)
	DeleteBookmark(id string) error
	ListBookmarkRefsByTeam(teamID string) ([]bookmarkmodel.BookmarkRef, error)
	ListBookmarkRefsByChannel(channelID string) ([]bookmarkmodel.BookmarkRef, error)
}

// Option configures a Store.
type Option func(*KVStore)

// WithClock replaces the timestamp source, for tests.
func WithClock(now func() int64) Option {
	return func(s *KVStore) {
		s.now = now
	}
}

// KVStore is the plugin-KV implementation of Store.
type KVStore struct {
	kv    KV
	now   func() int64
	locks [lockStripes]sync.Mutex
}

// NewStore returns a Store backed by the live Mattermost plugin KV store.
func NewStore(client *pluginapi.Client) *KVStore {
	return NewStoreWithKV(pluginKV{client: client})
}

// NewStoreWithKV returns a Store backed by an arbitrary KV implementation.
func NewStoreWithKV(kv KV, opts ...Option) *KVStore {
	s := &KVStore{kv: kv, now: model.GetMillis}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

var _ Store = (*KVStore)(nil)

func bookmarkKey(id string) string {
	return "bookmark:" + id
}

func indexKey(prefix, scopeID string, shard int) string {
	return prefix + scopeID + ":" + strconv.Itoa(shard)
}

func shardFor(id string) int {
	return int(fnv32a(id) % indexShards)
}

func fnv32a(value string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= prime32
	}
	return hash
}

func (s *KVStore) lock(key string) *sync.Mutex {
	return &s.locks[fnv32a(key)%lockStripes]
}

func (s *KVStore) GetBookmark(id string) (*bookmarkmodel.Bookmark, error) {
	var bookmark bookmarkmodel.Bookmark
	if err := s.kv.Get(bookmarkKey(id), &bookmark); err != nil {
		return nil, err
	}
	if bookmark.ID == "" {
		return nil, ErrNotFound
	}
	if bookmark.Tags == nil {
		bookmark.Tags = []string{}
	}
	return &bookmark, nil
}

func (s *KVStore) CreateBookmark(bookmark *bookmarkmodel.Bookmark) error {
	lock := s.lock(bookmarkKey(bookmark.ID))
	lock.Lock()
	defer lock.Unlock()

	var existing []byte
	if err := s.kv.Get(bookmarkKey(bookmark.ID), &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		return ErrExists
	}
	if bookmark.Version == 0 {
		bookmark.Version = 1
	}
	if _, err := s.kv.Set(bookmarkKey(bookmark.ID), bookmark); err != nil {
		return err
	}
	if err := s.syncBookmarkRefs(nil, bookmark); err != nil {
		_ = s.kv.Delete(bookmarkKey(bookmark.ID))
		_ = s.syncBookmarkRefs(bookmark, nil)
		return fmt.Errorf("failed to update bookmark indexes: %w", err)
	}
	return nil
}

func (s *KVStore) UpdateBookmark(id string, mutate func(current *bookmarkmodel.Bookmark) (*bookmarkmodel.Bookmark, error)) (*bookmarkmodel.Bookmark, error) {
	lock := s.lock(bookmarkKey(id))
	lock.Lock()
	defer lock.Unlock()

	for range casRetries {
		var raw []byte
		if err := s.kv.Get(bookmarkKey(id), &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, ErrNotFound
		}
		var current bookmarkmodel.Bookmark
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, fmt.Errorf("failed to decode bookmark: %w", err)
		}
		next, err := mutate(current.Clone())
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, ErrNotFound
		}
		next.ID = current.ID
		next.CreatorID = current.CreatorID
		next.CreatedAt = current.CreatedAt
		next.Version = current.Version + 1
		next.UpdatedAt = s.now()
		if next.Tags == nil {
			next.Tags = []string{}
		}

		saved, err := s.kv.CompareAndSet(bookmarkKey(id), raw, next)
		if err != nil {
			return nil, err
		}
		if !saved {
			continue
		}
		if err := s.syncBookmarkRefs(&current, next); err != nil {
			return next, fmt.Errorf("failed to update bookmark indexes: %w", err)
		}
		return next, nil
	}
	return nil, bookmarkmodel.ErrConflict
}

func (s *KVStore) DeleteBookmark(id string) error {
	lock := s.lock(bookmarkKey(id))
	lock.Lock()
	defer lock.Unlock()

	var raw []byte
	if err := s.kv.Get(bookmarkKey(id), &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return ErrNotFound
	}
	var current bookmarkmodel.Bookmark
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("failed to decode bookmark: %w", err)
	}
	if err := s.kv.Delete(bookmarkKey(id)); err != nil {
		return err
	}
	if err := s.syncBookmarkRefs(&current, nil); err != nil {
		return fmt.Errorf("failed to update bookmark indexes: %w", err)
	}
	return nil
}

func (s *KVStore) ListBookmarkRefsByTeam(teamID string) ([]bookmarkmodel.BookmarkRef, error) {
	return s.readBookmarkRefs("bookmark_team:", teamID)
}

func (s *KVStore) ListBookmarkRefsByChannel(channelID string) ([]bookmarkmodel.BookmarkRef, error) {
	return s.readBookmarkRefs("bookmark_channel:", channelID)
}

func (s *KVStore) readBookmarkRefs(prefix, scopeID string) ([]bookmarkmodel.BookmarkRef, error) {
	refs := make([]bookmarkmodel.BookmarkRef, 0, 32)
	for shard := range indexShards {
		var shardRefs []bookmarkmodel.BookmarkRef
		if err := s.kv.Get(indexKey(prefix, scopeID, shard), &shardRefs); err != nil {
			return nil, fmt.Errorf("failed to read bookmark index shard: %w", err)
		}
		refs = append(refs, shardRefs...)
	}
	return refs, nil
}

func (s *KVStore) syncBookmarkRefs(oldBookmark, nextBookmark *bookmarkmodel.Bookmark) error {
	if oldBookmark != nil && nextBookmark != nil {
		oldValue, nextValue := oldBookmark.Ref(), nextBookmark.Ref()
		if bookmarkRefFieldsEqual(&oldValue, &nextValue) {
			return nil
		}
	}
	var errs []error

	oldTeam, newTeam := "", ""
	oldRef := bookmarkmodel.BookmarkRef{}
	newRef := bookmarkmodel.BookmarkRef{}
	if oldBookmark != nil {
		oldTeam = oldBookmark.TeamID
		oldRef = oldBookmark.Ref()
	}
	if nextBookmark != nil {
		newTeam = nextBookmark.TeamID
		newRef = nextBookmark.Ref()
	}

	switch {
	case oldTeam != newTeam:
		if oldTeam != "" {
			if err := s.removeBookmarkRef("bookmark_team:", oldTeam, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newTeam != "" {
			if err := s.upsertBookmarkRef("bookmark_team:", newTeam, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldBookmark != nil && nextBookmark != nil:
		if err := s.upsertBookmarkRef("bookmark_team:", newTeam, newRef); err != nil {
			errs = append(errs, err)
		}
	}

	oldChannel, newChannel := "", ""
	if oldBookmark != nil {
		oldChannel = oldBookmark.ChannelID
	}
	if nextBookmark != nil {
		newChannel = nextBookmark.ChannelID
	}
	switch {
	case oldChannel != newChannel:
		if oldChannel != "" {
			if err := s.removeBookmarkRef("bookmark_channel:", oldChannel, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newChannel != "" {
			if err := s.upsertBookmarkRef("bookmark_channel:", newChannel, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldBookmark != nil && nextBookmark != nil:
		if err := s.upsertBookmarkRef("bookmark_channel:", newChannel, newRef); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *KVStore) upsertBookmarkRef(prefix, scopeID string, ref bookmarkmodel.BookmarkRef) error {
	if scopeID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(ref.ID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeBookmarkRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != ref.ID {
				continue
			}
			if refs[i].Version > ref.Version || bookmarkRefFieldsEqual(&refs[i], &ref) {
				return nil, ErrNoChange
			}
			refs[i] = ref
			return refs, nil
		}
		refs = append(refs, ref)
		return refs, nil
	})
}

func (s *KVStore) removeBookmarkRef(prefix, scopeID, bookmarkID string) error {
	if scopeID == "" || bookmarkID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(bookmarkID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeBookmarkRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != bookmarkID {
				continue
			}
			refs = append(refs[:i], refs[i+1:]...)
			return refs, nil
		}
		return nil, ErrNoChange
	})
}

func decodeBookmarkRefs(raw []byte) ([]bookmarkmodel.BookmarkRef, error) {
	if len(raw) == 0 {
		return []bookmarkmodel.BookmarkRef{}, nil
	}
	var refs []bookmarkmodel.BookmarkRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("failed to decode bookmark index: %w", err)
	}
	if refs == nil {
		refs = []bookmarkmodel.BookmarkRef{}
	}
	return refs, nil
}

func bookmarkRefFieldsEqual(a, b *bookmarkmodel.BookmarkRef) bool {
	return a.TeamID == b.TeamID &&
		a.ChannelID == b.ChannelID &&
		a.URL == b.URL &&
		a.Title == b.Title &&
		a.CreatorID == b.CreatorID &&
		a.SourcePostID == b.SourcePostID &&
		a.CreatedAt == b.CreatedAt &&
		slices.Equal(a.Tags, b.Tags)
}
