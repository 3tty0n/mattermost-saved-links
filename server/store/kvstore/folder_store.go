package kvstore

import (
	"encoding/json"
	"errors"
	"fmt"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

func (s *KVStore) GetFolder(id string) (*savedlinkmodel.Folder, error) {
	var folder savedlinkmodel.Folder
	if err := s.kv.Get(folderKey(id), &folder); err != nil {
		return nil, err
	}
	if folder.ID == "" {
		return nil, ErrNotFound
	}
	return &folder, nil
}

func (s *KVStore) CreateFolder(folder *savedlinkmodel.Folder) error {
	lock := s.lock(folderKey(folder.ID))
	lock.Lock()
	defer lock.Unlock()

	var existing []byte
	if err := s.kv.Get(folderKey(folder.ID), &existing); err != nil {
		return err
	}
	if len(existing) > 0 {
		return ErrExists
	}
	if folder.Version == 0 {
		folder.Version = 1
	}
	if _, err := s.kv.Set(folderKey(folder.ID), folder); err != nil {
		return err
	}
	if err := s.syncFolderRefs(nil, folder); err != nil {
		_ = s.kv.Delete(folderKey(folder.ID))
		_ = s.syncFolderRefs(folder, nil)
		return fmt.Errorf("failed to update folder indexes: %w", err)
	}
	return nil
}

func (s *KVStore) UpdateFolder(id string, mutate func(current *savedlinkmodel.Folder) (*savedlinkmodel.Folder, error)) (*savedlinkmodel.Folder, error) {
	lock := s.lock(folderKey(id))
	lock.Lock()
	defer lock.Unlock()

	for range casRetries {
		var raw []byte
		if err := s.kv.Get(folderKey(id), &raw); err != nil {
			return nil, err
		}
		if len(raw) == 0 {
			return nil, ErrNotFound
		}
		var current savedlinkmodel.Folder
		if err := json.Unmarshal(raw, &current); err != nil {
			return nil, fmt.Errorf("failed to decode folder: %w", err)
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

		saved, err := s.kv.CompareAndSet(folderKey(id), raw, next)
		if err != nil {
			return nil, err
		}
		if !saved {
			continue
		}
		if err := s.syncFolderRefs(&current, next); err != nil {
			return next, fmt.Errorf("failed to update folder indexes: %w", err)
		}
		return next, nil
	}
	return nil, savedlinkmodel.ErrConflict
}

func (s *KVStore) DeleteFolder(id string) error {
	lock := s.lock(folderKey(id))
	lock.Lock()
	defer lock.Unlock()

	var raw []byte
	if err := s.kv.Get(folderKey(id), &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return ErrNotFound
	}
	var current savedlinkmodel.Folder
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("failed to decode folder: %w", err)
	}
	if err := s.kv.Delete(folderKey(id)); err != nil {
		return err
	}
	if err := s.syncFolderRefs(&current, nil); err != nil {
		return fmt.Errorf("failed to update folder indexes: %w", err)
	}
	return nil
}

func (s *KVStore) ListFolderRefsByTeam(teamID string) ([]savedlinkmodel.FolderRef, error) {
	return s.readFolderRefs("folder_team:", teamID)
}

func (s *KVStore) ListFolderRefsByChannel(channelID string) ([]savedlinkmodel.FolderRef, error) {
	return s.readFolderRefs("folder_channel:", channelID)
}

func (s *KVStore) readFolderRefs(prefix, scopeID string) ([]savedlinkmodel.FolderRef, error) {
	refs := make([]savedlinkmodel.FolderRef, 0, 16)
	for shard := range indexShards {
		var shardRefs []savedlinkmodel.FolderRef
		if err := s.kv.Get(indexKey(prefix, scopeID, shard), &shardRefs); err != nil {
			return nil, fmt.Errorf("failed to read folder index shard: %w", err)
		}
		refs = append(refs, shardRefs...)
	}
	return refs, nil
}

func (s *KVStore) syncFolderRefs(oldFolder, nextFolder *savedlinkmodel.Folder) error {
	if oldFolder != nil && nextFolder != nil {
		oldValue, nextValue := oldFolder.Ref(), nextFolder.Ref()
		if folderRefFieldsEqual(&oldValue, &nextValue) {
			return nil
		}
	}
	var errs []error

	oldTeam, newTeam := "", ""
	oldRef := savedlinkmodel.FolderRef{}
	newRef := savedlinkmodel.FolderRef{}
	if oldFolder != nil {
		oldTeam = oldFolder.TeamID
		oldRef = oldFolder.Ref()
	}
	if nextFolder != nil {
		newTeam = nextFolder.TeamID
		newRef = nextFolder.Ref()
	}

	switch {
	case oldTeam != newTeam:
		if oldTeam != "" {
			if err := s.removeFolderRef("folder_team:", oldTeam, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newTeam != "" {
			if err := s.upsertFolderRef("folder_team:", newTeam, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldFolder != nil && nextFolder != nil:
		if err := s.upsertFolderRef("folder_team:", newTeam, newRef); err != nil {
			errs = append(errs, err)
		}
	}

	oldChannel, newChannel := "", ""
	if oldFolder != nil {
		oldChannel = oldFolder.ChannelID
	}
	if nextFolder != nil {
		newChannel = nextFolder.ChannelID
	}
	switch {
	case oldChannel != newChannel:
		if oldChannel != "" {
			if err := s.removeFolderRef("folder_channel:", oldChannel, oldRef.ID); err != nil {
				errs = append(errs, err)
			}
		}
		if newChannel != "" {
			if err := s.upsertFolderRef("folder_channel:", newChannel, newRef); err != nil {
				errs = append(errs, err)
			}
		}
	case oldFolder != nil && nextFolder != nil:
		if err := s.upsertFolderRef("folder_channel:", newChannel, newRef); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *KVStore) upsertFolderRef(prefix, scopeID string, ref savedlinkmodel.FolderRef) error {
	if scopeID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(ref.ID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeFolderRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != ref.ID {
				continue
			}
			if refs[i].Version > ref.Version || folderRefFieldsEqual(&refs[i], &ref) {
				return nil, ErrNoChange
			}
			refs[i] = ref
			return refs, nil
		}
		refs = append(refs, ref)
		return refs, nil
	})
}

func (s *KVStore) removeFolderRef(prefix, scopeID, folderID string) error {
	if scopeID == "" || folderID == "" {
		return nil
	}
	key := indexKey(prefix, scopeID, shardFor(folderID))
	return s.kv.Update(key, func(oldValue []byte) (any, error) {
		refs, err := decodeFolderRefs(oldValue)
		if err != nil {
			return nil, err
		}
		for i := range refs {
			if refs[i].ID != folderID {
				continue
			}
			refs = append(refs[:i], refs[i+1:]...)
			return refs, nil
		}
		return nil, ErrNoChange
	})
}

func decodeFolderRefs(raw []byte) ([]savedlinkmodel.FolderRef, error) {
	if len(raw) == 0 {
		return []savedlinkmodel.FolderRef{}, nil
	}
	var refs []savedlinkmodel.FolderRef
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("failed to decode folder index: %w", err)
	}
	if refs == nil {
		refs = []savedlinkmodel.FolderRef{}
	}
	return refs, nil
}

func folderRefFieldsEqual(a, b *savedlinkmodel.FolderRef) bool {
	return a.TeamID == b.TeamID &&
		a.ChannelID == b.ChannelID &&
		a.Name == b.Name &&
		a.CreatorID == b.CreatorID &&
		a.CreatedAt == b.CreatedAt
}
