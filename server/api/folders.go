package api

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

type createFolderRequest struct {
	TeamID    string `json:"teamId"`
	ChannelID string `json:"channelId"`
	Name      string `json:"name"`
}

func (req createFolderRequest) input() savedlinkmodel.CreateFolderInput {
	return savedlinkmodel.CreateFolderInput{
		TeamID:    req.TeamID,
		ChannelID: req.ChannelID,
		Name:      req.Name,
	}
}

type updateFolderRequest struct {
	Name      *string `json:"name"`
	TeamID    *string `json:"teamId"`
	ChannelID *string `json:"channelId"`
}

func (req updateFolderRequest) patch() savedlinkmodel.FolderPatch {
	return savedlinkmodel.FolderPatch{Name: req.Name, TeamID: req.TeamID, ChannelID: req.ChannelID}
}

func (h *Handler) handleListFolders(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	result, err := h.savedLinks.ListFolders(actorID(r), firstValue(query, "team_id"), firstValue(query, "channel_id"))
	if err != nil {
		writeServiceError(w, r, err, "Failed to list folders")
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

func (h *Handler) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req createFolderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	folder, err := h.savedLinks.CreateFolder(pluginContext(r), r.URL.Path, actorID(r), req.input())
	if err != nil {
		writeServiceError(w, r, err, "Failed to create the folder")
		return
	}
	writeJSON(w, r, http.StatusCreated, folder)
}

func (h *Handler) handleGetFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := folderIDFromPath(w, r)
	if !ok {
		return
	}
	folder, err := h.savedLinks.GetFolder(actorID(r), id)
	if err != nil {
		writeServiceError(w, r, err, "Failed to load the folder")
		return
	}
	writeJSON(w, r, http.StatusOK, folder)
}

func (h *Handler) handleUpdateFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := folderIDFromPath(w, r)
	if !ok {
		return
	}
	var req updateFolderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "Invalid request body", nil)
		return
	}
	folder, err := h.savedLinks.UpdateFolder(pluginContext(r), r.URL.Path, actorID(r), id, req.patch())
	if err != nil {
		writeServiceError(w, r, err, "Failed to update the folder")
		return
	}
	writeJSON(w, r, http.StatusOK, folder)
}

func (h *Handler) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := folderIDFromPath(w, r)
	if !ok {
		return
	}
	if err := h.savedLinks.DeleteFolder(pluginContext(r), r.URL.Path, actorID(r), id); err != nil {
		writeServiceError(w, r, err, "Failed to delete the folder")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func folderIDFromPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["folder_id"]
	if !model.IsValidId(id) {
		writeError(w, r, http.StatusNotFound, "Not found", nil)
		return "", false
	}
	return id, true
}
