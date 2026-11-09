// Package model defines the saved link records shared by the server API and the webapp.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/mattermost/mattermost/server/public/model"
)

var (
	// ErrInvalid wraps user-correctable input errors. The API maps it to 400.
	ErrInvalid = errors.New("invalid input")

	// ErrForbidden is returned when the actor may not perform the operation.
	ErrForbidden = errors.New("forbidden")

	// ErrNotFound is returned when a record does not exist or must not be disclosed.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when a compare-and-swap loop cannot make progress.
	ErrConflict = errors.New("conflict")
)

// ErrorMessage returns the user-facing part of a validation error.
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	message = strings.TrimPrefix(message, ErrInvalid.Error()+": ")
	message = strings.TrimPrefix(message, ErrForbidden.Error()+": ")
	return message
}

const (
	MaxSavedLinkNameRunes = 200
	MaxTags               = 20
	MaxTagRunes           = 50
	MaxURLBytes           = 2048
)

// SavedLink is the full saved link record.
type SavedLink struct {
	ID           string   `json:"id"`
	TeamID       string   `json:"teamId"`
	ChannelID    string   `json:"channelId,omitempty"`
	FolderID     string   `json:"folderId,omitempty"`
	Position     int64    `json:"position"`
	URL          string   `json:"url"`
	Name         string   `json:"name"`
	Tags         []string `json:"tags"`
	CreatorID    string   `json:"creatorId"`
	SourcePostID string   `json:"sourcePostId,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
	Version      int64    `json:"version"`
}

// UnmarshalJSON accepts records written before the field was renamed from title to name. Old
// records stored a "title" key; new records only use "name".
func (b *SavedLink) UnmarshalJSON(data []byte) error {
	type savedLinkAlias SavedLink
	aux := struct {
		*savedLinkAlias
		LegacyTitle string `json:"title"`
	}{savedLinkAlias: (*savedLinkAlias)(b)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if b.Name == "" {
		b.Name = strings.TrimSpace(aux.LegacyTitle)
	}
	return nil
}

// Clone returns a deep copy.
func (b *SavedLink) Clone() *SavedLink {
	clone := *b
	clone.Tags = append([]string(nil), b.Tags...)
	return &clone
}

// SavedLinkRef is the compact index representation.
type SavedLinkRef struct {
	ID           string   `json:"i"`
	TeamID       string   `json:"t"`
	ChannelID    string   `json:"c,omitempty"`
	FolderID     string   `json:"f,omitempty"`
	Position     int64    `json:"p"`
	URL          string   `json:"l"`
	Name         string   `json:"n"`
	Tags         []string `json:"g,omitempty"`
	CreatorID    string   `json:"o"`
	SourcePostID string   `json:"x,omitempty"`
	CreatedAt    int64    `json:"k"`
	UpdatedAt    int64    `json:"u"`
	Version      int64    `json:"v"`
}

// Ref builds the index reference for a saved link.
func (b *SavedLink) Ref() SavedLinkRef {
	return SavedLinkRef{
		ID:           b.ID,
		TeamID:       b.TeamID,
		ChannelID:    b.ChannelID,
		FolderID:     b.FolderID,
		Position:     b.Position,
		URL:          b.URL,
		Name:         truncateRunes(b.Name, MaxSavedLinkNameRunes),
		Tags:         append([]string(nil), b.Tags...),
		CreatorID:    b.CreatorID,
		SourcePostID: b.SourcePostID,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
		Version:      b.Version,
	}
}

// SavedLinkSummary is the list representation sent to the webapp.
type SavedLinkSummary struct {
	ID           string   `json:"id"`
	TeamID       string   `json:"teamId"`
	ChannelID    string   `json:"channelId,omitempty"`
	FolderID     string   `json:"folderId,omitempty"`
	Position     int64    `json:"position"`
	URL          string   `json:"url"`
	Name         string   `json:"name"`
	Tags         []string `json:"tags"`
	CreatorID    string   `json:"creatorId"`
	SourcePostID string   `json:"sourcePostId,omitempty"`
	CreatedAt    int64    `json:"createdAt"`
	UpdatedAt    int64    `json:"updatedAt"`
}

// Summary converts an index reference to its API representation.
func (r SavedLinkRef) Summary() SavedLinkSummary {
	tags := append([]string{}, r.Tags...)
	return SavedLinkSummary{
		ID:           r.ID,
		TeamID:       r.TeamID,
		ChannelID:    r.ChannelID,
		FolderID:     r.FolderID,
		Position:     r.Position,
		URL:          r.URL,
		Name:         r.Name,
		Tags:         tags,
		CreatorID:    r.CreatorID,
		SourcePostID: r.SourcePostID,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

// CreateSavedLinkInput is the validated input to the saved link service.
type CreateSavedLinkInput struct {
	TeamID       string
	ChannelID    string
	FolderID     string
	URL          string
	Name         string
	Tags         []string
	SourcePostID string
}

// SavedLinkPatch is a partial update. A nil pointer means unchanged.
type SavedLinkPatch struct {
	TeamID       *string
	ChannelID    *string
	FolderID     *string
	URL          *string
	Name         *string
	Tags         *[]string
	SourcePostID *string
}

// IsZero reports whether the patch changes nothing.
func (p SavedLinkPatch) IsZero() bool {
	return p.TeamID == nil && p.ChannelID == nil && p.FolderID == nil && p.URL == nil &&
		p.Name == nil && p.Tags == nil && p.SourcePostID == nil
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// NormalizeURL validates a saved link URL. Only http(s) URLs are accepted; credentials in the URL
// are rejected so a saved link can never smuggle a password into a rendered link.
func NormalizeURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: URL is required", ErrInvalid)
	}
	if len(value) > MaxURLBytes {
		return "", fmt.Errorf("%w: URL is longer than %d bytes", ErrInvalid, MaxURLBytes)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("%w: URL is not valid", ErrInvalid)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: only http and https URLs are supported", ErrInvalid)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: URL must include a host", ErrInvalid)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%w: URLs with credentials are not allowed", ErrInvalid)
	}
	return parsed.String(), nil
}

// NormalizeSavedLinkName validates a saved link name. Names are optional: when the name is empty
// the service derives a readable one from the URL, so a saved link can be saved with a URL alone.
func NormalizeSavedLinkName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > MaxSavedLinkNameRunes {
		return "", fmt.Errorf("%w: name is longer than %d characters", ErrInvalid, MaxSavedLinkNameRunes)
	}
	return value, nil
}

// DeriveSavedLinkName builds a display name from a URL. It prefers the last path segment and
// falls back to the host, so the plugin never has to fetch a page to label a saved link.
func DeriveSavedLinkName(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return truncateRunes(strings.TrimSpace(value), MaxSavedLinkNameRunes)
	}

	segment := ""
	if trimmed := strings.Trim(parsed.Path, "/"); trimmed != "" {
		parts := strings.Split(trimmed, "/")
		segment = parts[len(parts)-1]
		if unescaped, unescapeErr := url.PathUnescape(segment); unescapeErr == nil {
			segment = unescaped
		}
	}

	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	candidate := strings.TrimSpace(segment)
	if candidate == "" {
		candidate = host
	}
	if candidate == "" {
		candidate = strings.TrimSpace(value)
	}
	candidate = strings.NewReplacer("-", " ", "_", " ").Replace(candidate)
	return truncateRunes(strings.Join(strings.Fields(candidate), " "), MaxSavedLinkNameRunes)
}

// NormalizeTags trims, de-duplicates (case-insensitively) and validates tags.
func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) > MaxTags {
		return nil, fmt.Errorf("%w: too many tags", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > MaxTagRunes {
			return nil, fmt.Errorf("%w: tags are limited to %d characters", ErrInvalid, MaxTagRunes)
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// ValidateSourcePost checks the ID shape of a source post.
func ValidateSourcePost(postID string) error {
	if postID != "" && !model.IsValidId(postID) {
		return fmt.Errorf("%w: invalid source post", ErrInvalid)
	}
	return nil
}
