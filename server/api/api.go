// Package api serves the bookmark plugin's HTTP routes under /plugins/{plugin_id}/api/v1.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/experimental/bot/logger"

	"github.com/3tty0n/mattermost-bookmarks/server/access"
	"github.com/3tty0n/mattermost-bookmarks/server/bookmarks"
	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

// Handler routes the requests the server forwards to the plugin's ServeHTTP hook.
type Handler struct {
	pluginAPI plugin.API
	client    *pluginapi.Client
	access    *access.Checker
	bookmarks *bookmarks.Service
	router    *mux.Router
}

// NewHandler builds the API router.
func NewHandler(pluginAPI plugin.API, client *pluginapi.Client, checker *access.Checker, bookmarksService *bookmarks.Service) *Handler {
	h := &Handler{
		pluginAPI: pluginAPI,
		client:    client,
		access:    checker,
		bookmarks: bookmarksService,
	}
	h.router = h.initRouter()
	return h
}

func (h *Handler) initRouter() *mux.Router {
	router := mux.NewRouter()

	router.Use(h.logRequests, h.recoverPanic, limitRequestBody, setSecurityHeaders)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.Use(h.MattermostAuthorizationRequired)

	apiRouter.HandleFunc("/bookmarks", h.handleListBookmarks).Methods(http.MethodGet)
	apiRouter.HandleFunc("/bookmarks", h.handleCreateBookmark).Methods(http.MethodPost)
	apiRouter.HandleFunc("/bookmarks/{bookmark_id}", h.handleGetBookmark).Methods(http.MethodGet)
	apiRouter.HandleFunc("/bookmarks/{bookmark_id}", h.handleUpdateBookmark).Methods(http.MethodPatch, http.MethodPost)
	apiRouter.HandleFunc("/bookmarks/{bookmark_id}", h.handleDeleteBookmark).Methods(http.MethodDelete)

	return router
}

type pluginContextKey struct{}

// ServeHTTP has the plugin hook's signature; the plugin's ServeHTTP passes its arguments through.
func (h *Handler) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), pluginContextKey{}, c)))
}

// MattermostAuthorizationRequired trusts Mattermost-User-ID because the server sets it only after
// authenticating the request. Requests without a session are rejected.
func (h *Handler) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Mattermost-User-ID") == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const maxRequestBodyBytes = 256 << 10

type loggerContextKey struct{}

func requestLogger(r *http.Request) logger.Logger {
	return r.Context().Value(loggerContextKey{}).(logger.Logger)
}

func (h *Handler) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log := logger.New(h.pluginAPI).With(logger.LogContext{
			"user_id": r.Header.Get("Mattermost-User-ID"),
		})
		r = r.WithContext(context.WithValue(r.Context(), loggerContextKey{}, log))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		// The query string is omitted because it may carry search text.
		log.With(logger.LogContext{
			"method":   r.Method,
			"path":     r.URL.Path,
			"status":   rec.status,
			"duration": time.Since(start).String(),
		}).Debugf("HTTP request")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (h *Handler) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if panicVal := recover(); panicVal != nil {
				requestLogger(r).With(logger.LogContext{
					"path":  r.URL.Path,
					"panic": fmt.Sprint(panicVal),
					"stack": string(debug.Stack()),
				}).Errorf("Recovered from panic in HTTP handler")
				if !rec.wroteHeader {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Bookmark titles, URLs and search results are private user data.
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil {
		requestLogger(r).WithError(err).Errorf("%s", message)
	}
	writeJSON(w, r, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		requestLogger(r).WithError(err).Warnf("Failed to write response")
	}
}

// writeServiceError maps domain errors to generic responses. Validation messages are the only
// ones taken from the error, because they are written for users and contain no internals.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error, failure string) {
	switch {
	case errors.Is(err, bookmarkmodel.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "Not found", nil)
	case errors.Is(err, bookmarkmodel.ErrForbidden):
		writeError(w, r, http.StatusForbidden, "You don't have permission to do that", nil)
	case errors.Is(err, bookmarkmodel.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, bookmarkmodel.ErrorMessage(err), nil)
	case errors.Is(err, bookmarkmodel.ErrConflict):
		writeError(w, r, http.StatusConflict, "The item changed, please try again", nil)
	default:
		writeError(w, r, http.StatusInternalServerError, failure, err)
	}
}

func decodeJSON(r *http.Request, out any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

func pluginContext(r *http.Request) *plugin.Context {
	c, _ := r.Context().Value(pluginContextKey{}).(*plugin.Context)
	return c
}

func actorID(r *http.Request) string {
	return r.Header.Get("Mattermost-User-ID")
}

func bookmarkIDFromPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := mux.Vars(r)["bookmark_id"]
	if !model.IsValidId(id) {
		writeError(w, r, http.StatusNotFound, "Not found", nil)
		return "", false
	}
	return id, true
}

func parsePage(query map[string][]string) (page, perPage int, err error) {
	page, err = parseIntParam(query, "page", 0)
	if err != nil {
		return 0, 0, err
	}
	perPage, err = parseIntParam(query, "per_page", bookmarkmodel.DefaultPageSize)
	if err != nil {
		return 0, 0, err
	}
	return page, perPage, nil
}

func parseIntParam(query map[string][]string, name string, fallback int) (int, error) {
	values, ok := query[name]
	if !ok || len(values) == 0 || values[0] == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(values[0])
	if err != nil {
		return 0, fmt.Errorf("%w: invalid %s", bookmarkmodel.ErrInvalid, strings.ReplaceAll(name, "_", "-"))
	}
	return value, nil
}

func firstValue(query map[string][]string, name string) string {
	values := query[name]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
