package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

// BookmarkScope is the set of bookmarks a list request covers.
type BookmarkScope string

const (
	BookmarkScopeTeam    BookmarkScope = "team"
	BookmarkScopeChannel BookmarkScope = "channel"
	BookmarkScopeAll     BookmarkScope = "all"
)

const (
	DefaultPageSize = 30
	MaxPageSize     = 100
)

// BookmarkQuery is a validated bookmark list query.
type BookmarkQuery struct {
	Scope     BookmarkScope
	TeamID    string
	ChannelID string
	Search    string
	Tag       string
	Page      int
	PerPage   int
}

// Normalize applies defaults and validates the query.
func (q *BookmarkQuery) Normalize() error {
	switch q.Scope {
	case "":
		q.Scope = BookmarkScopeTeam
	case BookmarkScopeTeam:
		if !model.IsValidId(q.TeamID) {
			return fmt.Errorf("%w: a valid team is required", ErrInvalid)
		}
	case BookmarkScopeChannel:
		if !model.IsValidId(q.ChannelID) {
			return fmt.Errorf("%w: a valid channel is required", ErrInvalid)
		}
	case BookmarkScopeAll:
	default:
		return fmt.Errorf("%w: unknown bookmark scope", ErrInvalid)
	}
	if q.TeamID != "" && !model.IsValidId(q.TeamID) {
		return fmt.Errorf("%w: invalid team", ErrInvalid)
	}
	if q.ChannelID != "" && !model.IsValidId(q.ChannelID) {
		return fmt.Errorf("%w: invalid channel", ErrInvalid)
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

// Match reports whether a bookmark reference satisfies the search filters.
func (q *BookmarkQuery) Match(ref BookmarkRef) bool {
	if q.Tag != "" && !hasTag(ref.Tags, q.Tag) {
		return false
	}
	if q.Search == "" {
		return true
	}
	search := strings.ToLower(q.Search)
	if strings.Contains(strings.ToLower(ref.Title), search) ||
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

// SortBookmarks orders refs newest first, then by ID for stable pagination.
func SortBookmarks(refs []BookmarkRef) {
	sort.Slice(refs, func(i, j int) bool {
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
