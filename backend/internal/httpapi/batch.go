package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

func (s Server) batchAuth(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if len(s.Config.Tokens) == 0 {
			fail(w, http.StatusNotFound, "Batch API disabled")
			return
		}
		scheme, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		caller := ""
		if strings.EqualFold(scheme, "Bearer") && token != "" {
			for name, expected := range s.Config.Tokens {
				if secureEqual(token, expected) {
					caller = name
				}
			}
		}
		if caller == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fail(w, http.StatusUnauthorized, "Invalid bearer token")
			return
		}
		next(w, r, caller)
	}
}

type batchLinks struct {
	store.Batch
	StatusURL  string `json:"status_url"`
	ResultsURL string `json:"results_url"`
}

type batchBasePathKey struct{}

func links(r *http.Request, batch store.Batch) batchLinks {
	prefix, _ := r.Context().Value(prefixKey{}).(string)
	base, _ := r.Context().Value(batchBasePathKey{}).(string)
	if base == "" {
		base = "/api/reverse/batches"
	}
	statusURL := prefix + base + "/" + batch.ID
	return batchLinks{Batch: batch, StatusURL: statusURL, ResultsURL: statusURL + "/results"}
}

func altviewBatchAccess(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r = r.WithContext(context.WithValue(r.Context(), batchBasePathKey{}, "/altview/place-batches"))
		next(w, r, "altview-mobile")
	}
}

func (s Server) submitBatch(w http.ResponseWriter, r *http.Request, caller string) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) < 1 || len(key) > 128 || strings.ContainsFunc(key, unicode.IsControl) {
		fail(w, http.StatusBadRequest, "Idempotency-Key must contain 1 to 128 bytes")
		return
	}
	var input struct {
		Points []pointInput `json:"points"`
	}
	if err := decodeBody(w, r, &input); err != nil {
		var overLimit *http.MaxBytesError
		if errors.As(err, &overLimit) {
			fail(w, http.StatusRequestEntityTooLarge, "Request body too large")
		} else {
			fail(w, http.StatusBadRequest, "Invalid request body")
		}
		return
	}
	if len(input.Points) < 1 || len(input.Points) > s.Config.BatchLimit {
		fail(w, http.StatusBadRequest, fmt.Sprintf("points must contain 1 to %d coordinates", s.Config.BatchLimit))
		return
	}
	points := make([]geocode.Point, len(input.Points))
	for i, item := range input.Points {
		p, valid := item.parse()
		if !valid {
			fail(w, http.StatusBadRequest, fmt.Sprintf("Invalid coordinates at index %d", i))
			return
		}
		points[i] = p
	}
	normalized, _ := json.Marshal(points)
	sum := fmt.Sprintf("%x", sha256.Sum256(normalized))
	ctx := r.Context()
	id, err := s.Store.SubmitBatch(ctx, caller, key, sum, points, s.Config.PendingLimit, s.Upstream.HasBatchSource())
	switch {
	case errors.Is(err, store.ErrBatchConflict):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrBatchQuota):
		w.Header().Set("Retry-After", "30")
		fail(w, http.StatusTooManyRequests, err.Error())
	case err != nil:
		batchError(w, err)
	default:
		select {
		case s.Wake <- struct{}{}:
		default:
		}
		batch, err := s.Store.Batch(ctx, caller, id)
		if err != nil {
			batchError(w, err)
			return
		}
		reply := links(r, batch)
		w.Header().Set("Location", reply.StatusURL)
		writeJSON(w, http.StatusAccepted, reply)
	}
}

func (s Server) batchFromRequest(w http.ResponseWriter, r *http.Request, caller string) (store.Batch, bool) {
	id := r.PathValue("id")
	if _, err := hex.DecodeString(id); len(id) != 32 || err != nil {
		fail(w, http.StatusNotFound, "Batch not found")
		return store.Batch{}, false
	}
	b, err := s.Store.Batch(r.Context(), caller, id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "Batch not found")
		return store.Batch{}, false
	}
	if err != nil {
		batchError(w, err)
		return store.Batch{}, false
	}
	return b, true
}

func (s Server) batchStatus(w http.ResponseWriter, r *http.Request, caller string) {
	if batch, ok := s.batchFromRequest(w, r, caller); ok {
		writeJSON(w, http.StatusOK, links(r, batch))
	}
}

func (s Server) batchResults(w http.ResponseWriter, r *http.Request, caller string) {
	batch, ok := s.batchFromRequest(w, r, caller)
	if !ok {
		return
	}
	page, limit, offset := pagination(r, 100, 500)
	items, err := s.Store.BatchItems(r.Context(), caller, batch.ID, limit, offset)
	if err != nil {
		batchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		batchLinks
		Page    int               `json:"page"`
		Limit   int               `json:"limit"`
		HasMore bool              `json:"has_more"`
		Items   []store.BatchItem `json:"items"`
	}{links(r, batch), page, limit, offset+limit < batch.Total, items})
}

func batchError(w http.ResponseWriter, err error) {
	slog.Error("批量接口失败", "error", err)
	fail(w, http.StatusInternalServerError, "Batch operation failed")
}
