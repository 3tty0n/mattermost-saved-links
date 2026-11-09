package model

import (
	"encoding/json"
	"strings"
	"testing"

	mmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "https", input: "https://example.com/path?q=1", want: "https://example.com/path?q=1"},
		{name: "http", input: "http://example.com", want: "http://example.com"},
		{name: "empty", input: "", wantErr: true},
		{name: "javascript", input: "javascript:alert(1)", wantErr: true},
		{name: "file", input: "file:///etc/passwd", wantErr: true},
		{name: "credentials", input: "https://user" + ":pass@example.com", wantErr: true}, //nolint:gosec // test fixture for credential rejection
		{name: "no host", input: "https:///path", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeSavedLinkName(t *testing.T) {
	name, err := NormalizeSavedLinkName("  Docs  ")
	require.NoError(t, err)
	assert.Equal(t, "Docs", name)

	name, err = NormalizeSavedLinkName("")
	require.NoError(t, err)
	assert.Empty(t, name)

	_, err = NormalizeSavedLinkName(strings.Repeat("x", MaxSavedLinkNameRunes+1))
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestDeriveSavedLinkName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "https://example.com/docs", want: "docs"},
		{input: "https://example.com/docs/", want: "docs"},
		{input: "https://www.example.com/getting-started", want: "getting started"},
		{input: "https://example.com/a/b_c?q=1", want: "b c"},
		{input: "https://example.com/", want: "example.com"},
		{input: "https://example.com", want: "example.com"},
		{input: "https://example.com/%E8%B3%87%E6%96%99", want: "資料"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, DeriveSavedLinkName(tt.input))
		})
	}
}

func TestNormalizeFolderName(t *testing.T) {
	name, err := NormalizeFolderName("  Work  ")
	require.NoError(t, err)
	assert.Equal(t, "Work", name)

	_, err = NormalizeFolderName("")
	assert.ErrorIs(t, err, ErrInvalid)
	_, err = NormalizeFolderName(strings.Repeat("x", MaxFolderNameRunes+1))
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestNormalizeTags(t *testing.T) {
	tags, err := NormalizeTags([]string{"Work", " work ", "urgent"})
	require.NoError(t, err)
	assert.Equal(t, []string{"Work", "urgent"}, tags)

	tags, err = NormalizeTags(nil)
	require.NoError(t, err)
	assert.NotNil(t, tags)
	assert.Empty(t, tags)

	_, err = NormalizeTags([]string{string(make([]rune, MaxTagRunes+1))})
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestSavedLinkQueryMatchAndSort(t *testing.T) {
	folderID := mmodel.NewId()
	query := SavedLinkQuery{Search: "docs", Tag: "work", FolderID: folderID, PerPage: 10}
	require.NoError(t, query.Normalize())
	ref := SavedLinkRef{Name: "Team docs", URL: "https://example.com", Tags: []string{"Work"}, FolderID: folderID}
	assert.True(t, query.Match(ref))
	assert.False(t, query.Match(SavedLinkRef{Name: "Team docs", URL: "https://example.com", Tags: []string{"work"}}))
	assert.False(t, query.Match(SavedLinkRef{Name: "Team docs", URL: "https://example.com", Tags: []string{"Work"}, FolderID: "other"}))

	query.FolderID = SavedLinkFolderNone
	assert.False(t, query.Match(ref))
	assert.True(t, query.Match(SavedLinkRef{Name: "Team docs", URL: "https://example.com", Tags: []string{"Work"}}))

	query.FolderID = "not-an-id"
	assert.ErrorIs(t, query.Normalize(), ErrInvalid)

	refs := []SavedLinkRef{
		{ID: "a", CreatedAt: 1, Name: "a"},
		{ID: "b", CreatedAt: 3, Name: "b"},
		{ID: "c", CreatedAt: 2, Name: "c"},
	}
	SortSavedLinks(refs)
	assert.Equal(t, []string{"b", "c", "a"}, []string{refs[0].ID, refs[1].ID, refs[2].ID})
}

func TestSortSavedLinksByPosition(t *testing.T) {
	refs := []SavedLinkRef{
		{ID: "a", Position: 3000, CreatedAt: 3},
		{ID: "b", Position: 1000, CreatedAt: 1},
		{ID: "c", Position: 2000, CreatedAt: 2},
	}
	SortSavedLinks(refs)
	assert.Equal(t, []string{"b", "c", "a"}, []string{refs[0].ID, refs[1].ID, refs[2].ID})

	// Legacy records without a position keep their newest-first order.
	refs = []SavedLinkRef{
		{ID: "old", Position: 0, CreatedAt: 1},
		{ID: "new", Position: 0, CreatedAt: 2},
	}
	SortSavedLinks(refs)
	assert.Equal(t, []string{"new", "old"}, []string{refs[0].ID, refs[1].ID})
}

func TestSummariesUseEmptySlicesForMissingTags(t *testing.T) {
	summary := SavedLinkRef{ID: "id"}.Summary()
	assert.NotNil(t, summary.Tags)
	assert.Empty(t, summary.Tags)
}

func TestPaginate(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	page, more := Paginate(items, 0, 2)
	assert.Equal(t, []int{1, 2}, page)
	assert.True(t, more)

	page, more = Paginate(items, 2, 2)
	assert.Equal(t, []int{5}, page)
	assert.False(t, more)

	page, more = Paginate(items, 3, 2)
	assert.Empty(t, page)
	assert.NotNil(t, page)
	assert.False(t, more)
}

func TestSavedLinkDecodesLegacyTitle(t *testing.T) {
	var savedLink SavedLink
	require.NoError(t, json.Unmarshal([]byte(`{"id":"id","title":"Legacy","url":"https://example.com"}`), &savedLink))
	assert.Equal(t, "Legacy", savedLink.Name)

	require.NoError(t, json.Unmarshal([]byte(`{"id":"id","title":"Legacy","name":"Current"}`), &savedLink))
	assert.Equal(t, "Current", savedLink.Name)
}

func TestSortFolderSummaries(t *testing.T) {
	folders := []FolderSummary{
		{ID: "b", Name: "beta", CreatedAt: 2},
		{ID: "a", Name: "Alpha", CreatedAt: 1},
		{ID: "c", Name: "beta", CreatedAt: 1},
	}
	SortFolderSummaries(folders)
	assert.Equal(t, []string{"a", "c", "b"}, []string{folders[0].ID, folders[1].ID, folders[2].ID})
}
