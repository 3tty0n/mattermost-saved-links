package api

import (
	"net/http"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

type createSavedLinkRequest struct {
	TeamID       string   `json:"teamId"`
	ChannelID    string   `json:"channelId"`
	FolderID     string   `json:"folderId"`
	URL          string   `json:"url"`
	Name         string   `json:"name"`
	Tags         []string `json:"tags"`
	SourcePostID string   `json:"sourcePostId"`
}

func (req createSavedLinkRequest) input() savedlinkmodel.CreateSavedLinkInput {
	return savedlinkmodel.CreateSavedLinkInput{
		TeamID:       req.TeamID,
		ChannelID:    req.ChannelID,
		FolderID:     req.FolderID,
		URL:          req.URL,
		Name:         req.Name,
		Tags:         req.Tags,
		SourcePostID: req.SourcePostID,
	}
}

type updateSavedLinkRequest struct {
	TeamID       *string   `json:"teamId"`
	ChannelID    *string   `json:"channelId"`
	FolderID     *string   `json:"folderId"`
	URL          *string   `json:"url"`
	Name         *string   `json:"name"`
	Tags         *[]string `json:"tags"`
	SourcePostID *string   `json:"sourcePostId"`
}

func (req updateSavedLinkRequest) patch() savedlinkmodel.SavedLinkPatch {
	return savedlinkmodel.SavedLinkPatch{
		TeamID:       req.TeamID,
		ChannelID:    req.ChannelID,
		FolderID:     req.FolderID,
		URL:          req.URL,
		Name:         req.Name,
		Tags:         req.Tags,
		SourcePostID: req.SourcePostID,
	}
}

func (h *Handler) handleListSavedLinks(w http.ResponseWriter, r *http.Request) {
	query, err := savedLinkQueryFromRequest(r)
	if err != nil {
		writeServiceError(w, r, err, "Failed to list saved links")
		return
	}
	result, err := h.savedLinks.List(actorID(r), query)
	if err != nil {
		writeServiceError(w, r, err, "Failed to list saved links")
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *Handler) handleCreateSavedLink(w http.ResponseWriter, r *http.Request) {
	var req createSavedLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	savedLink, err := h.savedLinks.Create(pluginContext(r), r.URL.Path, actorID(r), req.input())
	if err != nil {
		writeServiceError(w, r, err, "Failed to create the saved link")
		return
	}
	writeJSON(w, r, http.StatusCreated, savedLink)
}

func (h *Handler) handleGetSavedLink(w http.ResponseWriter, r *http.Request) {
	id, ok := savedLinkIDFromPath(w, r)
	if !ok {
		return
	}
	savedLink, err := h.savedLinks.Get(actorID(r), id)
	if err != nil {
		writeServiceError(w, r, err, "Failed to load the saved link")
		return
	}
	writeJSON(w, r, http.StatusOK, savedLink)
}

func (h *Handler) handleUpdateSavedLink(w http.ResponseWriter, r *http.Request) {
	id, ok := savedLinkIDFromPath(w, r)
	if !ok {
		return
	}
	var req updateSavedLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	savedLink, err := h.savedLinks.Update(pluginContext(r), r.URL.Path, actorID(r), id, req.patch())
	if err != nil {
		writeServiceError(w, r, err, "Failed to update the saved link")
		return
	}
	writeJSON(w, r, http.StatusOK, savedLink)
}

type reorderSavedLinksRequest struct {
	SavedLinkIDs []string `json:"savedLinkIds"`
}

func (h *Handler) handleReorderSavedLinks(w http.ResponseWriter, r *http.Request) {
	var req reorderSavedLinksRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	if err := h.savedLinks.Reorder(pluginContext(r), r.URL.Path, actorID(r), req.SavedLinkIDs); err != nil {
		writeServiceError(w, r, err, "Failed to reorder saved links")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteSavedLink(w http.ResponseWriter, r *http.Request) {
	id, ok := savedLinkIDFromPath(w, r)
	if !ok {
		return
	}
	if err := h.savedLinks.Delete(pluginContext(r), r.URL.Path, actorID(r), id); err != nil {
		writeServiceError(w, r, err, "Failed to delete the saved link")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func savedLinkQueryFromRequest(r *http.Request) (savedlinkmodel.SavedLinkQuery, error) {
	query := r.URL.Query()
	page, perPage, err := parsePage(query)
	if err != nil {
		return savedlinkmodel.SavedLinkQuery{}, err
	}
	return savedlinkmodel.SavedLinkQuery{
		Scope:     savedlinkmodel.SavedLinkScope(firstValue(query, "scope")),
		TeamID:    firstValue(query, "team_id"),
		ChannelID: firstValue(query, "channel_id"),
		FolderID:  firstValue(query, "folder_id"),
		Search:    firstValue(query, "q"),
		Tag:       firstValue(query, "tag"),
		Page:      page,
		PerPage:   perPage,
	}, nil
}
