package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"umberrelay/internal/app"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAPISummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.DashboardSummary()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAPIDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.db.ListDevicesWithStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) handleAPIDevice(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	dev, err := s.db.GetDevice(mac)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "device not found")
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (s *Server) handleAPIUpdateDevice(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	var body struct {
		Label string `json:"label"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}

	if err := app.UpdateDeviceLabel(s.db, mac, body.Label); err != nil {
		if errors.Is(err, app.ErrDeviceNotFound) {
			writeJSONError(w, http.StatusNotFound, "device not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIQueries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	deviceMAC := q.Get("device")
	domain := q.Get("domain")
	limit := 100
	offset := 0
	if v := q.Get("limit"); v != "" {
		n, err := parseBoundedInt(v, 1, 1000)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
			return
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSONError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = n
	}
	var from, to time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		to = t
	}
	if to.IsZero() {
		to = time.Now()
	}
	if !from.IsZero() && to.Before(from) {
		writeJSONError(w, http.StatusBadRequest, "to must be after from")
		return
	}

	queries, err := s.db.QueryLog(deviceMAC, domain, from, to, limit, offset)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, queries)
}

func (s *Server) handleAPIActivity(w http.ResponseWriter, r *http.Request) {
	deviceMAC := r.URL.Query().Get("device")

	buckets, err := s.db.HourlyActivity(deviceMAC)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type activityBucket struct {
		Timestamp int64 `json:"timestamp"`
		Total     int   `json:"total"`
		Tracker   int   `json:"tracker"`
	}

	response := make([]activityBucket, 0, len(buckets))
	for _, bucket := range buckets {
		response = append(response, activityBucket{
			Timestamp: bucket.Timestamp.Unix(),
			Total:     bucket.TotalCount,
			Tracker:   bucket.TrackerCount,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAPIDomains(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := parseBoundedInt(v, 1, 1000)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "limit must be between 1 and 1000")
			return
		}
		limit = n
	}
	domains, err := s.db.TopDomains(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleAPIGetSettings(w http.ResponseWriter, r *http.Request) {
	retention, _ := s.db.GetConfig("retention_days")
	retentionDays := 30
	if retention != "" {
		if n, err := strconv.Atoi(retention); err == nil {
			retentionDays = n
		}
	}
	refreshHours, _ := s.db.GetConfig("list_refresh_hours")
	listRefreshHours := 24
	if refreshHours != "" {
		if n, err := strconv.Atoi(refreshHours); err == nil {
			listRefreshHours = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_days":     retentionDays,
		"list_refresh_hours": listRefreshHours,
	})
}

func (s *Server) handleAPIUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RetentionDays    *int `json:"retention_days"`
		ListRefreshHours *int `json:"list_refresh_hours"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}

	err := app.UpdateSettings(s.db, s.classify, app.SettingsInput{
		RetentionDays:    body.RetentionDays,
		ListRefreshHours: body.ListRefreshHours,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIListLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.db.ListLists()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (s *Server) handleAPIAddList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string `json:"url"`
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}
	id, err := app.AddList(r.Context(), s.db, app.AddListInput{
		URL:      body.URL,
		Name:     body.Name,
		Category: body.Category,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.refreshClassificationAsync()
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handleAPIUpdateList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}
	if body.Enabled == nil {
		writeJSONError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := app.UpdateListEnabled(s.db, id, *body.Enabled); err != nil {
		if errors.Is(err, app.ErrListNotFound) {
			writeJSONError(w, http.StatusNotFound, "list not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.refreshClassificationAsync()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeleteList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	if err := app.DeleteList(s.db, id); err != nil {
		if errors.Is(err, app.ErrListNotFound) {
			writeJSONError(w, http.StatusNotFound, "list not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.refreshClassificationAsync()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIRefreshLists(w http.ResponseWriter, r *http.Request) {
	if s.classify == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "classify manager not available")
		return
	}

	sources, err := app.EnabledListSources(s.db)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	go app.RefreshListSources(context.Background(), s.classify, sources)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleAPISetOverride(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	var body struct {
		Category string `json:"category"`
	}
	if !decodeAPIJSON(w, r, &body) {
		return
	}
	if body.Category == "" {
		writeJSONError(w, http.StatusBadRequest, "category is required")
		return
	}
	if err := app.SetDomainOverride(s.db, s.classify, domain, body.Category); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeleteOverride(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if err := app.DeleteDomainOverride(s.db, s.classify, domain); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseBoundedInt(value string, min, max int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("value must be between %d and %d", min, max)
	}
	return n, nil
}

func parseBoolFormValue(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("enabled must be true or false")
	}
}

func (s *Server) refreshClassificationAsync() {
	if s.classify == nil {
		return
	}
	sources, err := app.EnabledListSources(s.db)
	if err != nil {
		return
	}
	go s.classify.Refresh(context.Background(), sources)
}
