package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/echoflying/nominatim-cache/backend/internal/geocode"
	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

func operationError(w http.ResponseWriter, err error, message string) {
	slog.Error(message, "error", err)
	adminError(w, http.StatusInternalServerError, message)
}

type cacheListItem struct {
	store.Cache
	geocode.Place
}

func (s Server) listCache(w http.ResponseWriter, r *http.Request) {
	page, limit, offset := pagination(r, 20, 100)
	query := r.URL.Query()
	rows, total, err := s.Store.ListCache(r.Context(), query.Get("search"), query.Get("order"), query.Get("sort"), limit, offset)
	if err != nil {
		operationError(w, err, "Failed to get cache list")
		return
	}
	data := make([]cacheListItem, 0, len(rows))
	for _, c := range rows {
		raw, _ := c.Original()
		if c.Address == nil && raw != nil {
			formatted, _ := json.Marshal(raw.Address())
			address := string(formatted)
			c.Address = &address
		}
		data = append(data, cacheListItem{Cache: c, Place: geocode.SelectPlace(raw)})
	}
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "data": data, "pagination": jsonObject{
		"page": page, "limit": limit, "total": total, "total_pages": (total + int64(limit) - 1) / int64(limit),
		"has_more": int64(offset+limit) < total,
	}})
}

func (s Server) detailCache(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.Cache(r.Context(), r.PathValue("key"))
	if errors.Is(err, sql.ErrNoRows) {
		adminError(w, http.StatusNotFound, "Cache entry not found")
		return
	}
	if err != nil {
		operationError(w, err, "Failed to get cache entry")
		return
	}
	raw, err := c.Original()
	if err != nil {
		operationError(w, err, "Failed to get cache entry")
		return
	}
	adminOK(w, struct {
		store.Cache
		geocode.Place
		Original geocode.Response `json:"nominatim_response"`
	}{c, geocode.SelectPlace(raw), raw})
}

func (s Server) deleteCache(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	count, err := s.Store.DeleteCache(r.Context(), key)
	if err != nil {
		operationError(w, err, "Failed to delete cache")
		return
	}
	if count == 0 {
		adminError(w, http.StatusNotFound, "Cache entry not found")
		return
	}
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "message": "Cache deleted", "deleted_key": key})
}

func (s Server) clearCache(w http.ResponseWriter, r *http.Request) {
	count, err := s.Store.ClearCache(r.Context())
	if err != nil {
		operationError(w, err, "Failed to clear cache")
		return
	}
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "message": "All caches cleared", "deleted_count": count})
}

func (s Server) addCache(w http.ResponseWriter, r *http.Request) {
	var input struct {
		pointInput
		DisplayName string         `json:"display_name"`
		Address     map[string]any `json:"address"`
	}
	if err := decodeBody(w, r, &input); err != nil {
		adminError(w, http.StatusBadRequest, "Missing required fields")
		return
	}
	point, valid := input.parse()
	if !valid || strings.TrimSpace(input.DisplayName) == "" {
		adminError(w, http.StatusBadRequest, "Missing required fields")
		return
	}
	if input.Address == nil {
		input.Address = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.Config.ReverseTimeout)
	defer cancel()
	release, err := s.Locks.Lock(ctx, point.Key())
	if err != nil {
		operationError(w, err, "Failed to add cache")
		return
	}
	defer release()
	updated, err := s.Store.ManualCache(ctx, point, input.DisplayName, input.Address)
	if err != nil {
		operationError(w, err, "Failed to add cache")
		return
	}
	message := "已添加到缓存"
	if updated {
		message = "已更新缓存"
	}
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "message": message, "cache_key": point.Key(), "overwritten": updated})
}

func (s Server) compare(w http.ResponseWriter, r *http.Request) {
	var input pointInput
	err := decodeBody(w, r, &input)
	point, valid := input.parse()
	if err != nil || !valid {
		adminError(w, http.StatusBadRequest, "Invalid coordinates")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.Config.ReverseTimeout)
	defer cancel()
	results := s.Upstream.Compare(ctx, point)
	success := 0
	for _, result := range results {
		if result.Success {
			success++
		}
	}
	lat, _ := strconv.ParseFloat(geocode.Fixed4(point.Lat), 64)
	lon, _ := strconv.ParseFloat(geocode.Fixed4(point.Lon), 64)
	writeJSON(w, http.StatusOK, jsonObject{"success": true,
		"coordinates": geocode.Point{Lat: lat, Lon: lon}, "results": results,
		"summary": jsonObject{"total": len(results), "success": success, "failed": len(results) - success},
	})
}

func (s Server) logs(w http.ResponseWriter, r *http.Request) {
	page, limit, offset := pagination(r, 100, 1000)
	var filter *int
	if raw := r.URL.Query().Get("cached"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || (value != 0 && value != 1) {
			adminError(w, http.StatusBadRequest, "Invalid cached filter")
			return
		}
		filter = &value
	}
	rows, total, err := s.Store.Logs(r.Context(), filter, s.Config.Location, limit, offset)
	if err != nil {
		operationError(w, err, "Failed to get logs")
		return
	}
	writeJSON(w, http.StatusOK, jsonObject{"success": true, "data": rows, "pagination": jsonObject{
		"page": page, "limit": limit, "total": total, "has_more": int64(offset+limit) < total,
	}})
}

func (s Server) stats(w http.ResponseWriter, r *http.Request) {
	data, err := s.Store.Stats(r.Context(), s.Started)
	if err != nil {
		operationError(w, err, "Failed to get stats")
		return
	}
	adminOK(w, data)
}

func (s Server) daily(w http.ResponseWriter, r *http.Request) {
	data, err := s.Store.Daily(r.Context(), time.Now().In(s.Config.Location))
	if err != nil {
		operationError(w, err, "Failed to get daily stats")
		return
	}
	adminOK(w, data)
}
