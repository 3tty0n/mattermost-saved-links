// Package kvstore persists saved links and folders in the Mattermost plugin KV store and
// maintains compact per-scope indexes for list views.
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

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

const (
	// Four shards bound each index value and spread compare-and-swap contention.
	indexShards = 4
	lockStripes = 64
	casRetries  = 5
)

// Store is the persistence interface used by the saved link service.
type Store interface {
	GetSavedLink(id string) (*savedlinkmodel.SavedLink, error)
	CreateSavedLink(savedLink *savedlinkmodel.SavedLink) error
	UpdateSavedLink(id string, mutate func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error)) (*savedlinkmodel.SavedLink, error)
	DeleteSavedLink(id string) error
	ListSavedLinkRefsByTeam(teamID string) ([]savedlinkmodel.SavedLinkRef, error)
	ListSavedLinkRefsByChannel(channelID string) ([]savedlinkmodel.SavedLinkRef, error)

	GetFolder(id string) (*savedlinkmodel.Folder, error)
	CreateFolder(folder *savedlinkmodel.Folder) error
	UpdateFolder(id string, mutate func(current *savedlinkmodel.Folder) (*savedlinkmodel.Folder, error)) (*savedlinkmodel.Folder, error)
	DeleteFolder(id string) error
	ListFolderRefsByTeam(teamID string) ([]savedlinkmodel.FolderRef, error)
	ListFolderRefsByChannel(channelID string) ([]savedlinkmodel.FolderRef, error)
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

func savedLinkKey(id string) string {
	return "saved_link:" + id
}

func folderKey(id string) string {
	return "folder:" + id
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

func (s *KVStore) GetSavedLink(id string) (*savedlinkmodel.SavedLink, error) {
	var savedLink savedlinkmodel.SavedLink
	if err := s.kv.Get(savedLinkKey(id), &savedLink); err != nil {
		return nil, err
	}
	if savedLink.ID == "" {
		return nil, ErrNotFound
	}
	if savedLink.Tags == nil {
		savedLink.Tags = []string{}
	}
	return &savedLink, nil
}

func (s *KVStore) CreateSavedLink(savedLink *savedlinkmodel.SavedLink) error {
	lock := s.lock(savedLinkKey(savedLink.ID))
	lock.Lock()
	defer lock.Unlock()

	var existing []byte
	if err := s.kv.Get(savedLinkKey(savedLink.ID), &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		return ErrExists
	}
	if savedLink.Version == 0 {
		savedLink.Version = 1
	}
	if _, err := s.kv.Set(savedLinkKey(savedLink.ID), savedLink); err != nil {
		return err
	}
	if err := s.syncSavedLinkRefs(nil, savedLink); err != nil {
		_ = s.kv.Delete(savedLinkKey(savedLink.ID))
		_ = s.syncSavedLinkRefs(savedLink, nil)
		return fmt.Errorf("failed to update saved link indexes: %w", err)
	}
	return nil
}

func (s *KVStore) UpdateSavedLink(id string, mutate func(current *savedlinkmodel.SavedLink) (*savedlinkmodel.SavedLink, error)) (*savedlinkmodel.SavedLink, error) {
	lock := s.lock(savedLinkKey(id))
	lock.Lock()
	defer lock.Unlock()

	for range casRetries {
		var raw []byte
		if err := s.kv.Get(savedLinkKey(id), &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, ErrNotFound
		}
		var current savedlinkmodel.SavedLink
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, fmt.Errorf("failed to decode saved link: %w", err)
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

		saved, err := s.kv.CompareAndSet(savedLinkKey(id), raw, next)
		if err != nil {
			return nil, err
		}
		if !saved {
			continue
		}
		if err := s.syncSavedLinkRefs(&current, next); err != nil {
			return next, fmt.Errorf("failed to update saved link indexes: %w", err)
		}
		return next, nil
	}
	return nil, savedlinkmodel.ErrConflict
}

func (s *KVStore) DeleteSavedLink(id string) error {
	lock := s.lock(savedLinkKey(id))
	lock.Lock()
	defer lock.Unlock()

	var raw []byte
	if err := s.kv.Get(savedLinkKey(id), &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return ErrNotFound
	}
	var current savedlinkmodel.SavedLink
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("failed to decode saved link: %w", err)
	}
	if err := s.kv.Delete(savedLinkKey(id)); err != nil {
		return err
	}
	if err := s.syncSavedLinkRefs(&current, nil); err != nil {
		return fmt.Errorf("failed to update saved link indexes: %w", err)
	}
	return nil
}

func (s *KVStore) ListSavedLinkRefsByTeam(teamID string) ([]savedlinkmodel.SavedLinkRef, error) {
	return s.readSavedLinkRefs("saved_link_team:", teamID)
}

func (s *KVStore) ListSavedLinkRefsByChannel(channelID string) ([]savedlinkmodel.SavedLinkRef, error) {
	return s.readSavedLinkRefs("saved_link_channel:", channelID)
}

func (s *KVStore) readSavedLinkRefs(prefix, scopeID string) ([]savedlinkmodel.SavedLinkRef, error) {
	refs := make([]savedlinkmodel.SavedLinkRef, 0, 32)
	for shard := range indexShards {
		var shardRefs []savedlinkmodel.SavedLinkRef
		if err := s.kv.Get(indexKey(prefix, scopeID, shard), &shardRefs); err != nil {
			return nil, fmt.Errorf("failed to read saved link index shard: %w", err)
		}
		refs = append(refs, shardRefs...)
	}
	return refs, nil
}

func (s *KVStore) syncSavedLinkRefs(oldSavedLink, nextSavedLink *savedlinkmodel.SavedLink) error {
	if oldSavedLink != nil && nextSavedLink != nil {
		oldValue, nextValue := oldSavedLink.Ref(), nextSavedLink.Ref()
		if savedLinkRefFieldsEqual(&oldValue, &nextValue) {
			return nil
		}
	}
	var errs []error

	oldTeam, newTeam := "", ""
	oldRef := savedlinkmodel.SavedLinkRef{}
	newRef := savedlinkmodel.SavedLinkRef{}
	if oldSavedLink != nil {
		oldTeam = oldSavedLink.TeamID
		oldRef = oldSavedLink.Ref()
	}
	if nextSavedLink != nil {
		newTeam = nextSavedLink.TeamID
		newRef = nextSavedLink.Ref()
	}

	switch {
	case oldTeam != newTeam:
		if oldTeam != "" {
			if err := s.removeSavedLinkRef("saved_link_team:", oldTeam, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newTeam != "" {
			if err := s.upsertSavedLinkRef("saved_link_team:", newTeam, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldSavedLink != nil && nextSavedLink != nil:
		if err := s.upsertSavedLinkRef("saved_link_team:", newTeam, newRef); err != nil {
			errs = append(errs, err)
		}
	}

	oldChannel, newChannel := "", ""
	if oldSavedLink != nil {
		oldChannel = oldSavedLink.ChannelID
	}
	if nextSavedLink != nil {
		newChannel = nextSavedLink.ChannelID
	}
	switch {
	case oldChannel != newChannel:
		if oldChannel != "" {
			if err := s.removeSavedLinkRef("saved_link_channel:", oldChannel, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newChannel != "" {
			if err := s.upsertSavedLinkRef("saved_link_channel:", newChannel, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldSavedLink != nil && nextSavedLink != nil:
		if err := s.upsertSavedLinkRef("saved_link_channel:", newChannel, newRef); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *KVStore) upsertSavedLinkRef(prefix, scopeID string, ref savedlinkmodel.SavedLinkRef) error {
	if scopeID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(ref.ID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeSavedLinkRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != ref.ID {
				continue
			}
			if refs[i].Version > ref.Version || savedLinkRefFieldsEqual(&refs[i], &ref) {
				return nil, ErrNoChange
			}
			refs[i] = ref
			return refs, nil
		}
		refs = append(refs, ref)
		return refs, nil
	})
}

func (s *KVStore) removeSavedLinkRef(prefix, scopeID, savedLinkID string) error {
	if scopeID == "" || savedLinkID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(savedLinkID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeSavedLinkRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != savedLinkID {
				continue
			}
			refs = append(refs[:i], refs[i+1:]...)
			return refs, nil
		}
		return nil, ErrNoChange
	})
}

func decodeSavedLinkRefs(raw []byte) ([]savedlinkmodel.SavedLinkRef, error) {
	if len(raw) == 0 {
		return []savedlinkmodel.SavedLinkRef{}, nil
	}
	var refs []savedlinkmodel.SavedLinkRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("failed to decode saved link index: %w", err)
	}
	if refs == nil {
		refs = []savedlinkmodel.SavedLinkRef{}
	}
	return refs, nil
}

func savedLinkRefFieldsEqual(a, b *savedlinkmodel.SavedLinkRef) bool {
	return a.TeamID == b.TeamID &&
		a.ChannelID == b.ChannelID &&
		a.FolderID == b.FolderID &&
		a.Position == b.Position &&
		a.URL == b.URL &&
		a.Name == b.Name &&
		a.CreatorID == b.CreatorID &&
		a.SourcePostID == b.SourcePostID &&
		a.CreatedAt == b.CreatedAt &&
		slices.Equal(a.Tags, b.Tags)
}
