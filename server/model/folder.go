package model

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// MaxFolderNameRunes bounds a folder name in the UI and the compact folder index.
	MaxFolderNameRunes = 100

	// MaxFoldersPerScope keeps folder lists and their index shards small.
	MaxFoldersPerScope = 100
)

// Folder groups saved links inside one team or channel. A folder shares the scope of its
// saved links: a channel folder can only contain channel saved links.
type Folder struct {
	ID        string `json:"id"`
	TeamID    string `json:"teamId"`
	ChannelID string `json:"channelId,omitempty"`
	Name      string `json:"name"`
	CreatorID string `json:"creatorId"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	Version   int64  `json:"version"`
}

// Clone returns a deep copy.
func (f *Folder) Clone() *Folder {
	clone := *f
	return &clone
}

// FolderRef is the compact folder index representation.
type FolderRef struct {
	ID        string `json:"i"`
	TeamID    string `json:"t"`
	ChannelID string `json:"c,omitempty"`
	Name      string `json:"n"`
	CreatorID string `json:"o"`
	CreatedAt int64  `json:"k"`
	UpdatedAt int64  `json:"u"`
	Version   int64  `json:"v"`
}

// Ref builds the index reference for a folder.
func (f *Folder) Ref() FolderRef {
	return FolderRef{
		ID:        f.ID,
		TeamID:    f.TeamID,
		ChannelID: f.ChannelID,
		Name:      truncateRunes(f.Name, MaxFolderNameRunes),
		CreatorID: f.CreatorID,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
		Version:   f.Version,
	}
}

// FolderSummary is a folder plus the number of saved links it currently contains.
type FolderSummary struct {
	ID             string `json:"id"`
	TeamID         string `json:"teamId"`
	ChannelID      string `json:"channelId,omitempty"`
	Name           string `json:"name"`
	CreatorID      string `json:"creatorId"`
	SavedLinkCount int    `json:"savedLinkCount"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

// Summary converts an index reference to its API representation.
func (r FolderRef) Summary(savedLinkCount int) FolderSummary {
	return FolderSummary{
		ID:             r.ID,
		TeamID:         r.TeamID,
		ChannelID:      r.ChannelID,
		Name:           r.Name,
		CreatorID:      r.CreatorID,
		SavedLinkCount: savedLinkCount,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

// CreateFolderInput is the validated input to create a folder.
type CreateFolderInput struct {
	TeamID    string
	ChannelID string
	Name      string
}

// FolderPatch is a partial folder update. A nil pointer means unchanged.
type FolderPatch struct {
	Name      *string
	TeamID    *string
	ChannelID *string
}

// IsZero reports whether the patch changes nothing.
func (p FolderPatch) IsZero() bool {
	return p.Name == nil && p.TeamID == nil && p.ChannelID == nil
}

// NormalizeFolderName trims a folder name and enforces its size limit.
func NormalizeFolderName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: folder name is required", ErrInvalid)
	}
	if utf8.RuneCountInString(value) > MaxFolderNameRunes {
		return "", fmt.Errorf("%w: folder name is longer than %d characters", ErrInvalid, MaxFolderNameRunes)
	}
	return value, nil
}

// SortFolderSummaries orders folders by name, then creation time for stable output.
func SortFolderSummaries(folders []FolderSummary) {
	sort.Slice(folders, func(i, j int) bool {
		left, right := strings.ToLower(folders[i].Name), strings.ToLower(folders[j].Name)
		if left != right {
			return left < right
		}
		if folders[i].CreatedAt != folders[j].CreatedAt {
			return folders[i].CreatedAt < folders[j].CreatedAt
		}
		return folders[i].ID < folders[j].ID
	})
}
