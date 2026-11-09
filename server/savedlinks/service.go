// Package savedlinks implements URL saved link use cases. Saved links follow Mattermost's team and
// channel permission model and never fetch remote metadata, so there is no SSRF surface.
package savedlinks

import (
	"errors"
	"fmt"

	mmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	"github.com/3tty0n/mattermost-saved-links/server/access"
	"github.com/3tty0n/mattermost-saved-links/server/model"
	"github.com/3tty0n/mattermost-saved-links/server/store/kvstore"
)

// Publisher sends WebSocket change events.
type Publisher interface {
	SavedLinkChanged(action string, savedLink *model.SavedLink)
	FolderChanged(action string, folder *model.Folder)
}

// Service is the saved link application service.
type Service struct {
	client    *pluginapi.Client
	store     kvstore.Store
	access    *access.Checker
	publisher Publisher
	now       func() int64
}

// Option configures a Service.
type Option func(*Service)

// WithClock replaces the timestamp source, for tests.
func WithClock(now func() int64) Option {
	return func(s *Service) {
		s.now = now
	}
}

// NewService returns a saved link service.
func NewService(client *pluginapi.Client, store kvstore.Store, checker *access.Checker, publisher Publisher, opts ...Option) *Service {
	s := &Service{
		client:    client,
		store:     store,
		access:    checker,
		publisher: publisher,
		now:       mmodel.GetMillis,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ListResult is a page of saved link summaries.
type ListResult struct {
	Items   []model.SavedLinkSummary `json:"items"`
	Total   int                      `json:"total"`
	Page    int                      `json:"page"`
	PerPage int                      `json:"perPage"`
	HasMore bool                     `json:"hasMore"`
}

// Create stores a saved link after checking the scope and any source post.
func (s *Service) Create(c *plugin.Context, path, actorID string, input model.CreateSavedLinkInput) (*model.SavedLink, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	rec := s.audit(c, "createSavedLink", actorID, path)
	defer s.recordAudit(rec)

	normalizedURL, err := model.NormalizeURL(input.URL)
	if err != nil {
		return nil, err
	}
	name, err := model.NormalizeSavedLinkName(input.Name)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = model.DeriveSavedLinkName(normalizedURL)
	}
	tags, err := model.NormalizeTags(input.Tags)
	if err != nil {
		return nil, err
	}
	if err = model.ValidateSourcePost(input.SourcePostID); err != nil {
		return nil, err
	}
	if input.FolderID != "" && !mmodel.IsValidId(input.FolderID) {
		return nil, fmt.Errorf("%w: invalid folder", model.ErrInvalid)
	}

	teamID, channelID := input.TeamID, input.ChannelID
	if input.SourcePostID != "" {
		postScope, resolveErr := s.access.ResolvePost(actorID, input.SourcePostID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if channelID != "" && channelID != postScope.ChannelID {
			return nil, fmt.Errorf("%w: the saved link scope must match the post's channel", model.ErrInvalid)
		}
		if teamID != "" && teamID != postScope.TeamID {
			return nil, fmt.Errorf("%w: the saved link team must match the post's team", model.ErrInvalid)
		}
		teamID, channelID = postScope.TeamID, postScope.ChannelID
	}

	scope, err := s.access.Resolve(actorID, teamID, channelID)
	if err != nil {
		return nil, err
	}
	folder, err := s.folderInScope(input.FolderID, scope.TeamID, scope.ChannelID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	savedLink := &model.SavedLink{
		ID:           mmodel.NewId(),
		TeamID:       scope.TeamID,
		ChannelID:    scope.ChannelID,
		FolderID:     folderID(folder),
		Position:     now,
		URL:          normalizedURL,
		Name:         name,
		Tags:         tags,
		CreatorID:    actorID,
		SourcePostID: input.SourcePostID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Version:      1,
	}

	addAuditIDs(rec, savedLink)
	if err := s.store.CreateSavedLink(savedLink); err != nil {
		if errors.Is(err, kvstore.ErrExists) {
			return nil, model.ErrConflict
		}
		return nil, err
	}
	rec.Success()
	s.publish("created", savedLink)
	return savedLink, nil
}

// Get returns a saved link the actor may view.
func (s *Service) Get(actorID, savedLinkID string) (*model.SavedLink, error) {
	if !mmodel.IsValidId(savedLinkID) {
		return nil, model.ErrNotFound
	}
	savedLink, err := s.store.GetSavedLink(savedLinkID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if !s.access.CanView(actorID, savedLink.TeamID, savedLink.ChannelID) {
		return nil, model.ErrNotFound
	}
	return savedLink, nil
}

// List returns a filtered, sorted and paginated page of savedlinks.
func (s *Service) List(actorID string, query model.SavedLinkQuery) (*ListResult, error) {
	if err := query.Normalize(); err != nil {
		return nil, err
	}
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}

	cache := s.access.Cache()
	var refs []model.SavedLinkRef

	switch query.Scope {
	case model.SavedLinkScopeTeam:
		if !cache.CanViewTeam(actorID, query.TeamID) {
			return nil, model.ErrForbidden
		}
		loaded, err := s.store.ListSavedLinkRefsByTeam(query.TeamID)
		if err != nil {
			return nil, err
		}
		refs = filterRefs(loaded, func(ref model.SavedLinkRef) bool {
			return ref.TeamID == query.TeamID && cache.CanView(actorID, ref.TeamID, ref.ChannelID)
		})

	case model.SavedLinkScopeChannel:
		scope, err := s.access.Resolve(actorID, "", query.ChannelID)
		if err != nil {
			return nil, err
		}
		cache.Seed(scope.TeamID, scope.ChannelID, true)
		loaded, err := s.store.ListSavedLinkRefsByChannel(query.ChannelID)
		if err != nil {
			return nil, err
		}
		refs = filterRefs(loaded, func(ref model.SavedLinkRef) bool {
			return ref.ChannelID == query.ChannelID && ref.TeamID == scope.TeamID
		})

	case model.SavedLinkScopeAll:
		teams, err := s.client.Team.List(pluginapi.FilterTeamsByUser(actorID))
		if err != nil {
			return nil, err
		}
		refs = make([]model.SavedLinkRef, 0, 64)
		for _, team := range teams {
			cache.Seed(team.Id, "", true)
			loaded, err := s.store.ListSavedLinkRefsByTeam(team.Id)
			if err != nil {
				return nil, err
			}
			refs = append(refs, filterRefs(loaded, func(ref model.SavedLinkRef) bool {
				return ref.TeamID == team.Id && cache.CanView(actorID, ref.TeamID, ref.ChannelID)
			})...)
		}
	}

	filtered := refs[:0]
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, ok := seen[ref.ID]; ok {
			continue
		}
		if !query.Match(ref) {
			continue
		}
		seen[ref.ID] = struct{}{}
		filtered = append(filtered, ref)
	}
	model.SortSavedLinks(filtered)
	page, hasMore := model.Paginate(filtered, query.Page, query.PerPage)

	items := make([]model.SavedLinkSummary, len(page))
	for i, ref := range page {
		items[i] = ref.Summary()
	}
	return &ListResult{
		Items:   items,
		Total:   len(filtered),
		Page:    query.Page,
		PerPage: query.PerPage,
		HasMore: hasMore,
	}, nil
}

// Update applies a partial patch. Only the creator or an administrator may edit a saved link, and
// the actor must still be able to read its scope.
func (s *Service) Update(c *plugin.Context, path, actorID, savedLinkID string, patch model.SavedLinkPatch) (*model.SavedLink, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	if !mmodel.IsValidId(savedLinkID) {
		return nil, model.ErrNotFound
	}
	if patch.IsZero() {
		return nil, fmt.Errorf("%w: no changes", model.ErrInvalid)
	}

	normalized, err := normalizePatch(patch)
	if err != nil {
		return nil, err
	}

	rec := s.audit(c, "updateSavedLink", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "saved_link_id", savedLinkID)

	next, err := s.store.UpdateSavedLink(savedLinkID, func(current *model.SavedLink) (*model.SavedLink, error) {
		if !s.access.CanView(actorID, current.TeamID, current.ChannelID) {
			return nil, model.ErrNotFound
		}
		if current.CreatorID != actorID && !s.access.IsAdmin(actorID) {
			return nil, model.ErrForbidden
		}

		newTeam, newChannel := current.TeamID, current.ChannelID
		if patch.TeamID != nil {
			newTeam = *patch.TeamID
		}
		if patch.ChannelID != nil {
			newChannel = *patch.ChannelID
		}
		scopeChanged := newTeam != current.TeamID || newChannel != current.ChannelID

		if scopeChanged || patch.SourcePostID != nil {
			scope, resolveErr := s.access.Resolve(actorID, newTeam, newChannel)
			if resolveErr != nil {
				return nil, resolveErr
			}
			newTeam, newChannel = scope.TeamID, scope.ChannelID
		}

		if patch.SourcePostID != nil && *patch.SourcePostID != "" {
			postScope, resolveErr := s.access.ResolvePost(actorID, *patch.SourcePostID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if postScope.TeamID != newTeam || postScope.ChannelID != newChannel {
				return nil, fmt.Errorf("%w: the post must be in the saved link's channel", model.ErrInvalid)
			}
		}
		if scopeChanged && current.SourcePostID != "" && (patch.SourcePostID == nil || *patch.SourcePostID != "") {
			postScope, resolveErr := s.access.ResolvePost(actorID, current.SourcePostID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if postScope.TeamID != newTeam || postScope.ChannelID != newChannel {
				return nil, fmt.Errorf("%w: a saved link with a source post can not move away from it", model.ErrInvalid)
			}
		}

		newFolderID := current.FolderID
		if patch.FolderID != nil {
			newFolderID = *patch.FolderID
		}
		if newFolderID != "" {
			folder, folderErr := s.folderInScope(newFolderID, newTeam, newChannel)
			if folderErr != nil {
				if patch.FolderID != nil {
					return nil, folderErr
				}
				// The saved link moved to another scope and left its folder behind.
				newFolderID = ""
			} else {
				newFolderID = folder.ID
			}
		}

		if normalized.name != nil {
			current.Name = *normalized.name
		}
		if normalized.url != nil {
			current.URL = *normalized.url
		}
		if normalized.tags != nil {
			current.Tags = *normalized.tags
		}
		if normalized.sourcePostID != nil {
			current.SourcePostID = *normalized.sourcePostID
		}
		if normalized.name != nil && *normalized.name == "" {
			// Clearing the name keeps URL-only saved links readable.
			current.Name = model.DeriveSavedLinkName(current.URL)
		}
		groupChanged := scopeChanged || newFolderID != current.FolderID
		current.FolderID = newFolderID
		if scopeChanged {
			current.TeamID = newTeam
			current.ChannelID = newChannel
		}
		if groupChanged {
			// A moved saved link always lands at the end of its new folder.
			current.Position = s.now()
		}
		return current, nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}

	addAuditIDs(rec, next)
	rec.Success()
	s.publish("updated", next)
	return next, nil
}

// Delete removes a saved link.
func (s *Service) Delete(c *plugin.Context, path, actorID, savedLinkID string) error {
	if !mmodel.IsValidId(actorID) {
		return model.ErrForbidden
	}
	if !mmodel.IsValidId(savedLinkID) {
		return model.ErrNotFound
	}

	rec := s.audit(c, "deleteSavedLink", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "saved_link_id", savedLinkID)

	current, err := s.store.GetSavedLink(savedLinkID)
	if err != nil {
		return mapStoreError(err)
	}
	if !s.access.CanView(actorID, current.TeamID, current.ChannelID) {
		return model.ErrNotFound
	}
	if current.CreatorID != actorID && !s.access.IsAdmin(actorID) {
		return model.ErrForbidden
	}

	addAuditIDs(rec, current)
	if err := s.store.DeleteSavedLink(savedLinkID); err != nil {
		return mapStoreError(err)
	}
	rec.Success()
	s.publish("deleted", current)
	return nil
}

type normalizedPatch struct {
	name         *string
	url          *string
	tags         *[]string
	sourcePostID *string
}

func normalizePatch(patch model.SavedLinkPatch) (*normalizedPatch, error) {
	out := &normalizedPatch{}
	if patch.Name != nil {
		value, err := model.NormalizeSavedLinkName(*patch.Name)
		if err != nil {
			return nil, err
		}
		out.name = &value
	}
	if patch.URL != nil {
		value, err := model.NormalizeURL(*patch.URL)
		if err != nil {
			return nil, err
		}
		out.url = &value
	}
	if patch.Tags != nil {
		value, err := model.NormalizeTags(*patch.Tags)
		if err != nil {
			return nil, err
		}
		out.tags = &value
	}
	if patch.SourcePostID != nil {
		if err := model.ValidateSourcePost(*patch.SourcePostID); err != nil {
			return nil, err
		}
		value := *patch.SourcePostID
		out.sourcePostID = &value
	}
	if patch.FolderID != nil && *patch.FolderID != "" && !mmodel.IsValidId(*patch.FolderID) {
		return nil, fmt.Errorf("%w: invalid folder", model.ErrInvalid)
	}
	return out, nil
}

func filterRefs[T any](refs []T, keep func(T) bool) []T {
	out := make([]T, 0, len(refs))
	for _, ref := range refs {
		if keep(ref) {
			out = append(out, ref)
		}
	}
	return out
}

func (s *Service) publish(action string, savedLink *model.SavedLink) {
	if s.publisher != nil {
		s.publisher.SavedLinkChanged(action, savedLink)
	}
}

func (s *Service) audit(c *plugin.Context, event, actorID, path string) *mmodel.AuditRecord {
	if c == nil {
		c = &plugin.Context{}
	}
	return plugin.MakeAuditRecordWithContext(event, mmodel.AuditStatusFail, c, actorID, path)
}

func (s *Service) recordAudit(rec *mmodel.AuditRecord) {
	s.client.Audit.Record(rec)
}

func addAuditIDs(rec *mmodel.AuditRecord, savedLink *model.SavedLink) {
	mmodel.AddEventParameterToAuditRec(rec, "saved_link_id", savedLink.ID)
	mmodel.AddEventParameterToAuditRec(rec, "team_id", savedLink.TeamID)
	mmodel.AddEventParameterToAuditRec(rec, "channel_id", savedLink.ChannelID)
	mmodel.AddEventParameterToAuditRec(rec, "folder_id", savedLink.FolderID)
}

func mapStoreError(err error) error {
	switch {
	case errors.Is(err, kvstore.ErrNotFound):
		return model.ErrNotFound
	case errors.Is(err, model.ErrConflict):
		return model.ErrConflict
	default:
		return err
	}
}
