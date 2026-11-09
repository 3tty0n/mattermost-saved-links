package savedlinks

import (
	"errors"
	"fmt"
	"strings"

	mmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/3tty0n/mattermost-saved-links/server/access"
	"github.com/3tty0n/mattermost-saved-links/server/model"
	"github.com/3tty0n/mattermost-saved-links/server/store/kvstore"
)

// FolderListResult is the list of folders in one team or channel scope.
type FolderListResult struct {
	Items []model.FolderSummary `json:"items"`
	Total int                   `json:"total"`
}

// CreateFolder adds a folder to a team or channel scope.
func (s *Service) CreateFolder(c *plugin.Context, path, actorID string, input model.CreateFolderInput) (*model.Folder, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	rec := s.audit(c, "createSavedLinkFolder", actorID, path)
	defer s.recordAudit(rec)

	name, err := model.NormalizeFolderName(input.Name)
	if err != nil {
		return nil, err
	}
	scope, err := s.access.Resolve(actorID, input.TeamID, input.ChannelID)
	if err != nil {
		return nil, err
	}
	refs, err := s.listFolderRefs(scope)
	if err != nil {
		return nil, err
	}
	if len(refs) >= model.MaxFoldersPerScope {
		return nil, fmt.Errorf("%w: a scope can have at most %d folders", model.ErrInvalid, model.MaxFoldersPerScope)
	}
	for _, ref := range refs {
		if strings.EqualFold(ref.Name, name) {
			return nil, fmt.Errorf("%w: a folder with this name already exists", model.ErrInvalid)
		}
	}

	now := s.now()
	folder := &model.Folder{
		ID:        mmodel.NewId(),
		TeamID:    scope.TeamID,
		ChannelID: scope.ChannelID,
		Name:      name,
		CreatorID: actorID,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   1,
	}
	addFolderAuditIDs(rec, folder)
	if err := s.store.CreateFolder(folder); err != nil {
		if errors.Is(err, kvstore.ErrExists) {
			return nil, model.ErrConflict
		}
		return nil, err
	}
	rec.Success()
	s.publishFolder("created", folder)
	return folder, nil
}

// GetFolder returns a folder the actor may view.
func (s *Service) GetFolder(actorID, folderID string) (*model.Folder, error) {
	if !mmodel.IsValidId(folderID) {
		return nil, model.ErrNotFound
	}
	folder, err := s.store.GetFolder(folderID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if !s.access.CanView(actorID, folder.TeamID, folder.ChannelID) {
		return nil, model.ErrNotFound
	}
	return folder, nil
}

// ListFolders returns the folders of one team or channel with their saved link counts.
func (s *Service) ListFolders(actorID, teamID, channelID string) (*FolderListResult, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	if teamID == "" && channelID == "" {
		return nil, fmt.Errorf("%w: a team or channel is required", model.ErrInvalid)
	}
	scope, err := s.access.Resolve(actorID, teamID, channelID)
	if err != nil {
		return nil, err
	}
	refs, err := s.listFolderRefs(scope)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(refs))
	if len(refs) > 0 {
		counts, err = s.savedLinkCountsByFolder(scope)
		if err != nil {
			return nil, err
		}
	}

	items := make([]model.FolderSummary, len(refs))
	for i, ref := range refs {
		items[i] = ref.Summary(counts[ref.ID])
	}
	model.SortFolderSummaries(items)
	return &FolderListResult{Items: items, Total: len(items)}, nil
}

// UpdateFolder renames a folder. Only its creator or an administrator may change it.
func (s *Service) UpdateFolder(c *plugin.Context, path, actorID, folderID string, patch model.FolderPatch) (*model.Folder, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	if !mmodel.IsValidId(folderID) {
		return nil, model.ErrNotFound
	}
	if patch.IsZero() {
		return nil, fmt.Errorf("%w: no changes", model.ErrInvalid)
	}

	var normalizedName *string
	if patch.Name != nil {
		name, err := model.NormalizeFolderName(*patch.Name)
		if err != nil {
			return nil, err
		}
		normalizedName = &name
	}

	rec := s.audit(c, "updateSavedLinkFolder", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "folder_id", folderID)

	next, err := s.store.UpdateFolder(folderID, func(current *model.Folder) (*model.Folder, error) {
		if !s.access.CanView(actorID, current.TeamID, current.ChannelID) {
			return nil, model.ErrNotFound
		}
		if current.CreatorID != actorID && !s.access.IsAdmin(actorID) {
			return nil, model.ErrForbidden
		}
		if normalizedName != nil {
			refs, listErr := s.listFolderRefs(&access.Scope{TeamID: current.TeamID, ChannelID: current.ChannelID})
			if listErr != nil {
				return nil, listErr
			}
			for _, ref := range refs {
				if ref.ID != current.ID && strings.EqualFold(ref.Name, *normalizedName) {
					return nil, fmt.Errorf("%w: a folder with this name already exists", model.ErrInvalid)
				}
			}
			current.Name = *normalizedName
		}
		return current, nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}

	addFolderAuditIDs(rec, next)
	rec.Success()
	s.publishFolder("updated", next)
	return next, nil
}

// DeleteFolder removes a folder and ungroups its savedlinks. The saved links themselves are kept.
func (s *Service) DeleteFolder(c *plugin.Context, path, actorID, folderID string) error {
	if !mmodel.IsValidId(actorID) {
		return model.ErrForbidden
	}
	if !mmodel.IsValidId(folderID) {
		return model.ErrNotFound
	}

	rec := s.audit(c, "deleteSavedLinkFolder", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "folder_id", folderID)

	current, err := s.store.GetFolder(folderID)
	if err != nil {
		return mapStoreError(err)
	}
	if !s.access.CanView(actorID, current.TeamID, current.ChannelID) {
		return model.ErrNotFound
	}
	if current.CreatorID != actorID && !s.access.IsAdmin(actorID) {
		return model.ErrForbidden
	}

	refs, err := s.savedLinkRefsInScope(&access.Scope{TeamID: current.TeamID, ChannelID: current.ChannelID})
	if err != nil {
		return err
	}
	var ungroupErrs []error
	for _, ref := range refs {
		if ref.FolderID != folderID {
			continue
		}
		updated, updateErr := s.store.UpdateSavedLink(ref.ID, func(savedLink *model.SavedLink) (*model.SavedLink, error) {
			if savedLink.FolderID == folderID {
				savedLink.FolderID = ""
			}
			return savedLink, nil
		})
		if updateErr != nil {
			ungroupErrs = append(ungroupErrs, fmt.Errorf("failed to ungroup saved link %s: %w", ref.ID, updateErr))
			continue
		}
		s.publish("updated", updated)
	}
	if len(ungroupErrs) > 0 {
		return errors.Join(ungroupErrs...)
	}

	addFolderAuditIDs(rec, current)
	if err := s.store.DeleteFolder(folderID); err != nil {
		return mapStoreError(err)
	}
	rec.Success()
	s.publishFolder("deleted", current)
	return nil
}

func (s *Service) folderInScope(folderID, teamID, channelID string) (*model.Folder, error) {
	if folderID == "" {
		return nil, nil
	}
	if !mmodel.IsValidId(folderID) {
		return nil, fmt.Errorf("%w: invalid folder", model.ErrInvalid)
	}
	folder, err := s.store.GetFolder(folderID)
	if err != nil {
		if errors.Is(err, kvstore.ErrNotFound) {
			return nil, fmt.Errorf("%w: folder not found", model.ErrInvalid)
		}
		return nil, err
	}
	if folder.TeamID != teamID || folder.ChannelID != channelID {
		return nil, fmt.Errorf("%w: the folder belongs to a different scope", model.ErrInvalid)
	}
	return folder, nil
}

func (s *Service) listFolderRefs(scope *access.Scope) ([]model.FolderRef, error) {
	var refs []model.FolderRef
	var err error
	if scope.ChannelID != "" {
		refs, err = s.store.ListFolderRefsByChannel(scope.ChannelID)
	} else {
		refs, err = s.store.ListFolderRefsByTeam(scope.TeamID)
	}
	if err != nil {
		return nil, err
	}
	return filterRefs(refs, func(ref model.FolderRef) bool {
		if scope.ChannelID != "" {
			return ref.TeamID == scope.TeamID && ref.ChannelID == scope.ChannelID
		}
		return ref.TeamID == scope.TeamID && ref.ChannelID == ""
	}), nil
}

func (s *Service) savedLinkRefsInScope(scope *access.Scope) ([]model.SavedLinkRef, error) {
	var refs []model.SavedLinkRef
	var err error
	if scope.ChannelID != "" {
		refs, err = s.store.ListSavedLinkRefsByChannel(scope.ChannelID)
	} else {
		refs, err = s.store.ListSavedLinkRefsByTeam(scope.TeamID)
	}
	if err != nil {
		return nil, err
	}
	return filterRefs(refs, func(ref model.SavedLinkRef) bool {
		if scope.ChannelID != "" {
			return ref.TeamID == scope.TeamID && ref.ChannelID == scope.ChannelID
		}
		return ref.TeamID == scope.TeamID && ref.ChannelID == ""
	}), nil
}

func (s *Service) savedLinkCountsByFolder(scope *access.Scope) (map[string]int, error) {
	refs, err := s.savedLinkRefsInScope(scope)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(refs))
	for _, ref := range refs {
		if ref.FolderID != "" {
			counts[ref.FolderID]++
		}
	}
	return counts, nil
}

func folderID(folder *model.Folder) string {
	if folder == nil {
		return ""
	}
	return folder.ID
}

func addFolderAuditIDs(rec *mmodel.AuditRecord, folder *model.Folder) {
	mmodel.AddEventParameterToAuditRec(rec, "folder_id", folder.ID)
	mmodel.AddEventParameterToAuditRec(rec, "team_id", folder.TeamID)
	mmodel.AddEventParameterToAuditRec(rec, "channel_id", folder.ChannelID)
}

func (s *Service) publishFolder(action string, folder *model.Folder) {
	if s.publisher != nil {
		s.publisher.FolderChanged(action, folder)
	}
}
