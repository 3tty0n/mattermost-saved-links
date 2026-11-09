package model

import (
	"testing"

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

func TestBookmarkQueryMatchAndSort(t *testing.T) {
	query := BookmarkQuery{Search: "docs", Tag: "work", PerPage: 10}
	require.NoError(t, query.Normalize())
	ref := BookmarkRef{Title: "Team docs", URL: "https://example.com", Tags: []string{"Work"}}
	assert.True(t, query.Match(ref))
	assert.False(t, query.Match(BookmarkRef{Title: "Team docs", URL: "https://example.com", Tags: []string{"home"}}))

	refs := []BookmarkRef{
		{ID: "a", CreatedAt: 1, Title: "a"},
		{ID: "b", CreatedAt: 3, Title: "b"},
		{ID: "c", CreatedAt: 2, Title: "c"},
	}
	SortBookmarks(refs)
	assert.Equal(t, []string{"b", "c", "a"}, []string{refs[0].ID, refs[1].ID, refs[2].ID})
}

func TestSummariesUseEmptySlicesForMissingTags(t *testing.T) {
	summary := BookmarkRef{ID: "id"}.Summary()
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
