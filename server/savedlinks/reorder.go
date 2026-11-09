package savedlinks

import (
	"fmt"

	mmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/3tty0n/mattermost-saved-links/server/model"
)

const (
	// maxReorderSavedLinks matches the largest page the list API returns, so the webapp can send
	// every saved link it shows in one reorder request.
	maxReorderSavedLinks = model.MaxPageSize

	// savedLinkPositionStep leaves room for a midpoint insert if a future version needs it.
	savedLinkPositionStep int64 = 1000
)

// Reorder stores a new manual order for saved links in one scope and folder. Any member who can
// view the group may sort it, because the saved link list is shared with the whole scope.
func (s *Service) Reorder(c *plugin.Context, path, actorID string, savedLinkIDs []string) error {
	if !mmodel.IsValidId(actorID) {
		return model.ErrForbidden
	}
	if len(savedLinkIDs) == 0 || len(savedLinkIDs) > maxReorderSavedLinks {
		return fmt.Errorf("%w: reorder between 1 and %d saved links", model.ErrInvalid, maxReorderSavedLinks)
	}
	seen := make(map[string]struct{}, len(savedLinkIDs))
	for _, id := range savedLinkIDs {
		if !mmodel.IsValidId(id) {
			return fmt.Errorf("%w: invalid saved link", model.ErrInvalid)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: duplicate saved link", model.ErrInvalid)
		}
		seen[id] = struct{}{}
	}

	first, err := s.store.GetSavedLink(savedLinkIDs[0])
	if err != nil {
		return mapStoreError(err)
	}
	if !s.access.CanView(actorID, first.TeamID, first.ChannelID) {
		return model.ErrNotFound
	}

	savedLinks := make([]*model.SavedLink, 0, len(savedLinkIDs))
	for _, id := range savedLinkIDs {
		savedLink, getErr := s.store.GetSavedLink(id)
		if getErr != nil {
			return mapStoreError(getErr)
		}
		if !sameSavedLinkGroup(first, savedLink) {
			return fmt.Errorf("%w: saved links must belong to the same scope and folder", model.ErrInvalid)
		}
		savedLinks = append(savedLinks, savedLink)
	}

	rec := s.audit(c, "reorderSavedLinks", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "team_id", first.TeamID)
	mmodel.AddEventParameterToAuditRec(rec, "channel_id", first.ChannelID)
	mmodel.AddEventParameterToAuditRec(rec, "folder_id", first.FolderID)
	mmodel.AddEventParameterToAuditRec(rec, "saved_link_count", len(savedLinks))

	for index, savedLink := range savedLinks {
		position := int64(index+1) * savedLinkPositionStep
		_, updateErr := s.store.UpdateSavedLink(savedLink.ID, func(current *model.SavedLink) (*model.SavedLink, error) {
			if !sameSavedLinkGroup(first, current) {
				return nil, model.ErrConflict
			}
			current.Position = position
			return current, nil
		})
		if updateErr != nil {
			return mapStoreError(updateErr)
		}
	}

	rec.Success()
	s.publish("reordered", first)
	return nil
}

func sameSavedLinkGroup(first, other *model.SavedLink) bool {
	return first.TeamID == other.TeamID &&
		first.ChannelID == other.ChannelID &&
		first.FolderID == other.FolderID
}
