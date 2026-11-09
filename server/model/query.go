package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// SavedLinkScope is the set of saved links a list request covers.
type SavedLinkScope string

const (
	SavedLinkScopeTeam    SavedLinkScope = "team"
	SavedLinkScopeChannel SavedLinkScope = "channel"
	SavedLinkScopeAll     SavedLinkScope = "all"
)

const (
	DefaultPageSize = 30
	MaxPageSize     = 100

	// SavedLinkFolderNone selects saved links that are not in any folder.
	SavedLinkFolderNone = "none"
)

// SavedLinkQuery is a validated saved link list query.
type SavedLinkQuery struct {
	Scope     SavedLinkScope
	TeamID    string
	ChannelID string
	FolderID  string
	Search    string
	Tag       string
	Page      int
	PerPage   int
}

// Normalize applies defaults and validates the query.
func (q *SavedLinkQuery) Normalize() error {
	switch q.Scope {
	case "":
		q.Scope = SavedLinkScopeTeam
	case SavedLinkScopeTeam:
		if !model.IsValidId(q.TeamID) {
			return fmt.Errorf("%w: a valid team is required", ErrInvalid)
		}
	case SavedLinkScopeChannel:
		if !model.IsValidId(q.ChannelID) {
			return fmt.Errorf("%w: a valid channel is required", ErrInvalid)
		}
	case SavedLinkScopeAll:
	default:
		return fmt.Errorf("%w: unknown saved link scope", ErrInvalid)
	}
	if q.TeamID != "" && !model.IsValidId(q.TeamID) {
		return fmt.Errorf("%w: invalid team", ErrInvalid)
	}
	if q.ChannelID != "" && !model.IsValidId(q.ChannelID) {
		return fmt.Errorf("%w: invalid channel", ErrInvalid)
	}
	q.FolderID = strings.TrimSpace(q.FolderID)
	if q.FolderID != "" && q.FolderID != SavedLinkFolderNone && !model.IsValidId(q.FolderID) {
		return fmt.Errorf("%w: invalid folder", ErrInvalid)
	}
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 200 {
		return fmt.Errorf("%w: search term is too long", ErrInvalid)
	}
	q.Tag = strings.TrimSpace(q.Tag)
	if len(q.Tag) > MaxTagRunes {
		return fmt.Errorf("%w: tag is too long", ErrInvalid)
	}
	if q.Page < 0 {
		return fmt.Errorf("%w: page must not be negative", ErrInvalid)
	}
	if q.PerPage == 0 {
		q.PerPage = DefaultPageSize
	}
	if q.PerPage < 1 || q.PerPage > MaxPageSize {
		return fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalid, MaxPageSize)
	}
	return nil
}

// Match reports whether a saved link reference satisfies the search filters.
func (q *SavedLinkQuery) Match(ref SavedLinkRef) bool {
	if q.FolderID == SavedLinkFolderNone {
		if ref.FolderID != "" {
			return false
		}
	} else if q.FolderID != "" && ref.FolderID != q.FolderID {
		return false
	}
	if q.Tag != "" && !hasTag(ref.Tags, q.Tag) {
		return false
	}
	if q.Search == "" {
		return true
	}
	search := strings.ToLower(q.Search)
	if strings.Contains(strings.ToLower(ref.Name), search) ||
		strings.Contains(strings.ToLower(ref.URL), search) {
		return true
	}
	for _, tag := range ref.Tags {
		if strings.Contains(strings.ToLower(tag), search) {
			return true
		}
	}
	return false
}

func hasTag(tags []string, tag string) bool {
	for _, candidate := range tags {
		if strings.EqualFold(candidate, tag) {
			return true
		}
	}
	return false
}

// SortSavedLinks orders refs by their saved position, then newest first for records that share a
// position (including legacy records, whose position is zero).
func SortSavedLinks(refs []SavedLinkRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Position != refs[j].Position {
			return refs[i].Position < refs[j].Position
		}
		if refs[i].CreatedAt != refs[j].CreatedAt {
			return refs[i].CreatedAt > refs[j].CreatedAt
		}
		return refs[i].ID > refs[j].ID
	})
}

// Paginate returns the requested page and whether more items follow.
func Paginate[T any](items []T, page, perPage int) ([]T, bool) {
	start := page * perPage
	if start >= len(items) {
		return []T{}, false
	}
	end := min(start+perPage, len(items))
	return items[start:end], end < len(items)
}
