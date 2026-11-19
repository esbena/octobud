package annotations

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	authsvc "github.com/octobud-hq/octobud/backend/internal/core/auth"
	"github.com/octobud-hq/octobud/backend/internal/db"
)

type Handler struct {
	store  db.Store
	auth   authsvc.AuthService
	logger *zap.Logger
}

func New(logger *zap.Logger, store db.Store, auth authsvc.AuthService) *Handler {
	return &Handler{store: store, auth: auth, logger: logger}
}

func (h *Handler) Register(r chi.Router) {
	r.Route("/annotations", func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.save)
		r.Put("/{id}", h.save)
		r.Delete("/{id}", h.delete)
	})
}

func (h *Handler) failure(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		helpers.WriteError(w, http.StatusNotFound, "annotation not found")
		return
	}
	h.logger.Error("annotation operation failed", zap.Error(err))
	helpers.WriteError(w, http.StatusInternalServerError, "annotation operation failed")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.auth)
	if !ok {
		return
	}
	items, err := h.store.ListAnnotations(r.Context(), userID)
	if err != nil {
		h.failure(w, err)
		return
	}
	if items == nil {
		items = []db.Annotation{}
	}
	helpers.WriteJSON(w, http.StatusOK, map[string]any{"annotations": items})
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.auth)
	if !ok {
		return
	}
	var input struct {
		Name    string   `json:"name"`
		Code    string   `json:"code"`
		ViewIDs []string `json:"viewIds"`
		Enabled *bool    `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "invalid annotation body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		helpers.WriteError(w, http.StatusBadRequest, "expected a single annotation object")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || strings.TrimSpace(input.Code) == "" {
		helpers.WriteError(w, http.StatusBadRequest, "name and code are required")
		return
	}
	viewIDs := make([]string, 0, len(input.ViewIDs))
	seen := map[string]bool{}
	for _, id := range input.ViewIDs {
		if strings.TrimSpace(id) == "" {
			helpers.WriteError(w, http.StatusBadRequest, "view identifiers must not be empty")
			return
		}
		if !seen[id] {
			viewIDs = append(viewIDs, id)
			seen[id] = true
		}
	}
	item := db.Annotation{
		ID: chi.URLParam(r, "id"), Name: input.Name, Code: input.Code, ViewIDs: viewIDs, Enabled: true,
	}
	if input.Enabled != nil {
		item.Enabled = *input.Enabled
	}
	var result db.Annotation
	var err error
	status := http.StatusOK
	if r.Method == http.MethodPost {
		item.ID = uuid.NewString()
		result, err = h.store.CreateAnnotation(r.Context(), userID, item)
		status = http.StatusCreated
	} else {
		result, err = h.store.UpdateAnnotation(r.Context(), userID, item)
	}
	if err != nil {
		h.failure(w, err)
		return
	}
	helpers.WriteJSON(w, status, map[string]any{"annotation": result})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.auth)
	if !ok {
		return
	}
	if err := h.store.DeleteAnnotation(r.Context(), userID, chi.URLParam(r, "id")); err != nil {
		h.failure(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
