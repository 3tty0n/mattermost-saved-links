package api

import (
	"net/http"

	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

type createBookmarkRequest struct {
	TeamID       string   `json:"teamId"`
	ChannelID    string   `json:"channelId"`
	URL          string   `json:"url"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	SourcePostID string   `json:"sourcePostId"`
}

func (req createBookmarkRequest) input() bookmarkmodel.CreateBookmarkInput {
	return bookmarkmodel.CreateBookmarkInput{
		TeamID:       req.TeamID,
		ChannelID:    req.ChannelID,
		URL:          req.URL,
		Title:        req.Title,
		Description:  req.Description,
		Tags:         req.Tags,
		SourcePostID: req.SourcePostID,
	}
}

type updateBookmarkRequest struct {
	TeamID       *string   `json:"teamId"`
	ChannelID    *string   `json:"channelId"`
	URL          *string   `json:"url"`
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	Tags         *[]string `json:"tags"`
	SourcePostID *string   `json:"sourcePostId"`
}

func (req updateBookmarkRequest) patch() bookmarkmodel.BookmarkPatch {
	return bookmarkmodel.BookmarkPatch{
		TeamID:       req.TeamID,
		ChannelID:    req.ChannelID,
		URL:          req.URL,
		Title:        req.Title,
		Description:  req.Description,
		Tags:         req.Tags,
		SourcePostID: req.SourcePostID,
	}
}

func (h *Handler) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	query, err := bookmarkQueryFromRequest(r)
	if err != nil {
		writeServiceError(w, r, err, "Failed to list bookmarks")
		return
	}
	result, err := h.bookmarks.List(actorID(r), query)
	if err != nil {
		writeServiceError(w, r, err, "Failed to list bookmarks")
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *Handler) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var req createBookmarkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	bookmark, err := h.bookmarks.Create(pluginContext(r), r.URL.Path, actorID(r), req.input())
	if err != nil {
		writeServiceError(w, r, err, "Failed to create the bookmark")
		return
	}
	writeJSON(w, r, http.StatusCreated, bookmark)
}

func (h *Handler) handleGetBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := bookmarkIDFromPath(w, r)
	if !ok {
		return
	}
	bookmark, err := h.bookmarks.Get(actorID(r), id)
	if err != nil {
		writeServiceError(w, r, err, "Failed to load the bookmark")
		return
	}
	writeJSON(w, r, http.StatusOK, bookmark)
}

func (h *Handler) handleUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := bookmarkIDFromPath(w, r)
	if !ok {
		return
	}
	var req updateBookmarkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	bookmark, err := h.bookmarks.Update(pluginContext(r), r.URL.Path, actorID(r), id, req.patch())
	if err != nil {
		writeServiceError(w, r, err, "Failed to update the bookmark")
		return
	}
	writeJSON(w, r, http.StatusOK, bookmark)
}

func (h *Handler) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := bookmarkIDFromPath(w, r)
	if !ok {
		return
	}
	if err := h.bookmarks.Delete(pluginContext(r), r.URL.Path, actorID(r), id); err != nil {
		writeServiceError(w, r, err, "Failed to delete the bookmark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func bookmarkQueryFromRequest(r *http.Request) (bookmarkmodel.BookmarkQuery, error) {
	query := r.URL.Query()
	page, perPage, err := parsePage(query)
	if err != nil {
		return bookmarkmodel.BookmarkQuery{}, err
	}
	return bookmarkmodel.BookmarkQuery{
		Scope:     bookmarkmodel.BookmarkScope(firstValue(query, "scope")),
		TeamID:    firstValue(query, "team_id"),
		ChannelID: firstValue(query, "channel_id"),
		Search:    firstValue(query, "q"),
		Tag:       firstValue(query, "tag"),
		Page:      page,
		PerPage:   perPage,
	}, nil
}
