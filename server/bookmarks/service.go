// Package bookmarks implements URL bookmark use cases. Bookmarks follow Mattermost's team and
// channel permission model and never fetch remote metadata, so there is no SSRF surface.
package bookmarks

import (
	"errors"
	"fmt"

	mmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	"github.com/3tty0n/mattermost-bookmarks/server/access"
	"github.com/3tty0n/mattermost-bookmarks/server/model"
	"github.com/3tty0n/mattermost-bookmarks/server/store/kvstore"
)

// Publisher sends WebSocket change events.
type Publisher interface {
	BookmarkChanged(action string, bookmark *model.Bookmark)
}

// Service is the bookmark application service.
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

// NewService returns a bookmark service.
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

// ListResult is a page of bookmark summaries.
type ListResult struct {
	Items   []model.BookmarkSummary `json:"items"`
	Total   int                     `json:"total"`
	Page    int                     `json:"page"`
	PerPage int                     `json:"perPage"`
	HasMore bool                    `json:"hasMore"`
}

// Create stores a bookmark after checking the scope and any source post.
func (s *Service) Create(c *plugin.Context, path, actorID string, input model.CreateBookmarkInput) (*model.Bookmark, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	rec := s.audit(c, "createBookmark", actorID, path)
	defer s.recordAudit(rec)

	normalizedURL, err := model.NormalizeURL(input.URL)
	if err != nil {
		return nil, err
	}
	title, err := model.NormalizeBookmarkTitle(input.Title)
	if err != nil {
		return nil, err
	}
	description, err := model.NormalizeDescription(input.Description)
	if err != nil {
		return nil, err
	}
	tags, err := model.NormalizeTags(input.Tags)
	if err != nil {
		return nil, err
	}
	if err = model.ValidateSourcePost(input.SourcePostID); err != nil {
		return nil, err
	}

	teamID, channelID := input.TeamID, input.ChannelID
	if input.SourcePostID != "" {
		postScope, resolveErr := s.access.ResolvePost(actorID, input.SourcePostID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if channelID != "" && channelID != postScope.ChannelID {
			return nil, fmt.Errorf("%w: the bookmark scope must match the post's channel", model.ErrInvalid)
		}
		if teamID != "" && teamID != postScope.TeamID {
			return nil, fmt.Errorf("%w: the bookmark team must match the post's team", model.ErrInvalid)
		}
		teamID, channelID = postScope.TeamID, postScope.ChannelID
	}

	scope, err := s.access.Resolve(actorID, teamID, channelID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	bookmark := &model.Bookmark{
		ID:           mmodel.NewId(),
		TeamID:       scope.TeamID,
		ChannelID:    scope.ChannelID,
		URL:          normalizedURL,
		Title:        title,
		Description:  description,
		Tags:         tags,
		CreatorID:    actorID,
		SourcePostID: input.SourcePostID,
		CreatedAt:    now,
		UpdatedAt:    now,
		Version:      1,
	}

	addAuditIDs(rec, bookmark)
	if err := s.store.CreateBookmark(bookmark); err != nil {
		if errors.Is(err, kvstore.ErrExists) {
			return nil, model.ErrConflict
		}
		return nil, err
	}
	rec.Success()
	s.publish("created", bookmark)
	return bookmark, nil
}

// Get returns a bookmark the actor may view.
func (s *Service) Get(actorID, bookmarkID string) (*model.Bookmark, error) {
	if !mmodel.IsValidId(bookmarkID) {
		return nil, model.ErrNotFound
	}
	bookmark, err := s.store.GetBookmark(bookmarkID)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if !s.access.CanView(actorID, bookmark.TeamID, bookmark.ChannelID) {
		return nil, model.ErrNotFound
	}
	return bookmark, nil
}

// List returns a filtered, sorted and paginated page of bookmarks.
func (s *Service) List(actorID string, query model.BookmarkQuery) (*ListResult, error) {
	if err := query.Normalize(); err != nil {
		return nil, err
	}
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}

	cache := s.access.Cache()
	var refs []model.BookmarkRef

	switch query.Scope {
	case model.BookmarkScopeTeam:
		if !cache.CanViewTeam(actorID, query.TeamID) {
			return nil, model.ErrForbidden
		}
		loaded, err := s.store.ListBookmarkRefsByTeam(query.TeamID)
		if err != nil {
			return nil, err
		}
		refs = filterRefs(loaded, func(ref model.BookmarkRef) bool {
			return ref.TeamID == query.TeamID && cache.CanView(actorID, ref.TeamID, ref.ChannelID)
		})

	case model.BookmarkScopeChannel:
		scope, err := s.access.Resolve(actorID, "", query.ChannelID)
		if err != nil {
			return nil, err
		}
		cache.Seed(scope.TeamID, scope.ChannelID, true)
		loaded, err := s.store.ListBookmarkRefsByChannel(query.ChannelID)
		if err != nil {
			return nil, err
		}
		refs = filterRefs(loaded, func(ref model.BookmarkRef) bool {
			return ref.ChannelID == query.ChannelID && ref.TeamID == scope.TeamID
		})

	case model.BookmarkScopeAll:
		teams, err := s.client.Team.List(pluginapi.FilterTeamsByUser(actorID))
		if err != nil {
			return nil, err
		}
		refs = make([]model.BookmarkRef, 0, 64)
		for _, team := range teams {
			cache.Seed(team.Id, "", true)
			loaded, err := s.store.ListBookmarkRefsByTeam(team.Id)
			if err != nil {
				return nil, err
			}
			refs = append(refs, filterRefs(loaded, func(ref model.BookmarkRef) bool {
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
	model.SortBookmarks(filtered)
	page, hasMore := model.Paginate(filtered, query.Page, query.PerPage)

	items := make([]model.BookmarkSummary, len(page))
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

// Update applies a partial patch. Only the creator or an administrator may edit a bookmark, and
// the actor must still be able to read its scope.
func (s *Service) Update(c *plugin.Context, path, actorID, bookmarkID string, patch model.BookmarkPatch) (*model.Bookmark, error) {
	if !mmodel.IsValidId(actorID) {
		return nil, model.ErrForbidden
	}
	if !mmodel.IsValidId(bookmarkID) {
		return nil, model.ErrNotFound
	}
	if patch.IsZero() {
		return nil, fmt.Errorf("%w: no changes", model.ErrInvalid)
	}

	normalized, err := normalizePatch(patch)
	if err != nil {
		return nil, err
	}

	rec := s.audit(c, "updateBookmark", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "bookmark_id", bookmarkID)

	next, err := s.store.UpdateBookmark(bookmarkID, func(current *model.Bookmark) (*model.Bookmark, error) {
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
				return nil, fmt.Errorf("%w: the post must be in the bookmark's channel", model.ErrInvalid)
			}
		}
		if scopeChanged && current.SourcePostID != "" && (patch.SourcePostID == nil || *patch.SourcePostID != "") {
			postScope, resolveErr := s.access.ResolvePost(actorID, current.SourcePostID)
			if resolveErr != nil {
				return nil, resolveErr
			}
			if postScope.TeamID != newTeam || postScope.ChannelID != newChannel {
				return nil, fmt.Errorf("%w: a bookmark with a source post can not move away from it", model.ErrInvalid)
			}
		}

		if normalized.title != nil {
			current.Title = *normalized.title
		}
		if normalized.description != nil {
			current.Description = *normalized.description
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
		if scopeChanged {
			current.TeamID = newTeam
			current.ChannelID = newChannel
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

// Delete removes a bookmark.
func (s *Service) Delete(c *plugin.Context, path, actorID, bookmarkID string) error {
	if !mmodel.IsValidId(actorID) {
		return model.ErrForbidden
	}
	if !mmodel.IsValidId(bookmarkID) {
		return model.ErrNotFound
	}

	rec := s.audit(c, "deleteBookmark", actorID, path)
	defer s.recordAudit(rec)
	mmodel.AddEventParameterToAuditRec(rec, "bookmark_id", bookmarkID)

	current, err := s.store.GetBookmark(bookmarkID)
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
	if err := s.store.DeleteBookmark(bookmarkID); err != nil {
		return mapStoreError(err)
	}
	rec.Success()
	s.publish("deleted", current)
	return nil
}

type normalizedPatch struct {
	title        *string
	description  *string
	url          *string
	tags         *[]string
	sourcePostID *string
}

func normalizePatch(patch model.BookmarkPatch) (*normalizedPatch, error) {
	out := &normalizedPatch{}
	if patch.Title != nil {
		value, err := model.NormalizeBookmarkTitle(*patch.Title)
		if err != nil {
			return nil, err
		}
		out.title = &value
	}
	if patch.Description != nil {
		value, err := model.NormalizeDescription(*patch.Description)
		if err != nil {
			return nil, err
		}
		out.description = &value
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
	return out, nil
}

func filterRefs(refs []model.BookmarkRef, keep func(model.BookmarkRef) bool) []model.BookmarkRef {
	out := make([]model.BookmarkRef, 0, len(refs))
	for _, ref := range refs {
		if keep(ref) {
			out = append(out, ref)
		}
	}
	return out
}

func (s *Service) publish(action string, bookmark *model.Bookmark) {
	if s.publisher != nil {
		s.publisher.BookmarkChanged(action, bookmark)
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

func addAuditIDs(rec *mmodel.AuditRecord, bookmark *model.Bookmark) {
	mmodel.AddEventParameterToAuditRec(rec, "bookmark_id", bookmark.ID)
	mmodel.AddEventParameterToAuditRec(rec, "team_id", bookmark.TeamID)
	mmodel.AddEventParameterToAuditRec(rec, "channel_id", bookmark.ChannelID)
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
