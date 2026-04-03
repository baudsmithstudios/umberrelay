package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"umberrelay/internal/app"
	"umberrelay/internal/store"
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

func (s *Server) handleAPIActors(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	devices, err := s.db.ListDevicesWithTrendsAt(now)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sources, err := s.db.ListSourceWithTrendsAt(now)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type actorResponse struct {
		Key            string  `json:"key"`
		Type           string  `json:"type"`
		Name           string  `json:"name"`
		DeviceMAC      string  `json:"device_mac"`
		SourceIP       string  `json:"source_ip"`
		QueryCount     int     `json:"query_count"`
		TrackerPercent float64 `json:"tracker_percent"`
	}

	actors := make([]actorResponse, 0, len(devices)+len(sources))
	for _, device := range devices {
		actors = append(actors, actorResponse{
			Key:            actorKeyForDevice(device.MAC),
			Type:           actorTypeDevice,
			Name:           deviceDisplayName(device.Device),
			DeviceMAC:      device.MAC,
			QueryCount:     device.QueryCount,
			TrackerPercent: device.TrackerPercent,
		})
	}
	for _, source := range sources {
		actors = append(actors, actorResponse{
			Key:            actorKeyForSource(source.SourceIP),
			Type:           actorTypeSource,
			Name:           sourceActorDisplayName(source.SourceIP),
			SourceIP:       source.SourceIP,
			QueryCount:     source.QueryCount,
			TrackerPercent: source.TrackerPercent,
		})
	}

	sort.SliceStable(actors, func(i, j int) bool {
		if actors[i].TrackerPercent == actors[j].TrackerPercent {
			return actors[i].Name < actors[j].Name
		}
		return actors[i].TrackerPercent > actors[j].TrackerPercent
	})

	writeJSON(w, http.StatusOK, actors)
}

func (s *Server) handleAPIDevice(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	dev, err := s.db.GetDevice(mac)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "device not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
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
	actorKey := q.Get("actor")
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

	var queries []store.Query
	var err error
	if actorKey != "" {
		actorType, actorValue, ok := parseActorKey(actorKey)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "actor must be device:{mac} or source:{ip}")
			return
		}
		if actorType == actorTypeSource {
			queries, err = s.db.QueryLogBySource(actorValue, domain, from, to, limit, offset)
		} else {
			queries, err = s.db.QueryLog(actorValue, domain, from, to, limit, offset)
		}
	} else {
		queries, err = s.db.QueryLog(deviceMAC, domain, from, to, limit, offset)
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, queries)
}

func (s *Server) handleAPIActivity(w http.ResponseWriter, r *http.Request) {
	actorKey := r.URL.Query().Get("actor")
	deviceMAC := r.URL.Query().Get("device")
	sourceIP := r.URL.Query().Get("source")
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "24h"
	}

	var buckets []store.HourlyBucket
	var err error
	switch {
	case actorKey != "":
		actorType, actorValue, ok := parseActorKey(actorKey)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "actor must be device:{mac} or source:{ip}")
			return
		}
		if actorType == actorTypeSource {
			buckets, err = s.db.SourceRangedActivity(actorValue, timeRange)
		} else {
			buckets, err = s.db.RangedActivity(actorValue, timeRange)
		}
	case sourceIP != "":
		buckets, err = s.db.SourceRangedActivity(sourceIP, timeRange)
	default:
		buckets, err = s.db.RangedActivity(deviceMAC, timeRange)
	}
	if err != nil {
		if errors.Is(err, store.ErrInvalidRange) {
			writeJSONError(w, http.StatusBadRequest, "range must be one of 24h, 7d, 30d")
			return
		}
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
	domains, err := s.db.TopDomainsWithSource(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	stats, err := s.db.DashboardSummary()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		TotalDevices int                      `json:"total_devices"`
		Domains      []store.DomainWithSource `json:"domains"`
	}{
		TotalDevices: stats.DeviceCount,
		Domains:      domains,
	})
}

func (s *Server) handleAPIAnomalies(w http.ResponseWriter, r *http.Request) {
	anomalies, err := s.db.DeviceAnomalies()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type anomalyResponse struct {
		DeviceMAC           string  `json:"device_mac"`
		DeviceName          string  `json:"device_name"`
		Type                string  `json:"type"`
		CurrentValue        float64 `json:"current_value"`
		AverageValue        float64 `json:"average_value"`
		Delta               float64 `json:"delta"`
		TopDomain           string  `json:"top_domain"`
		TopDomainCategory   string  `json:"top_domain_category"`
		TopDomainSourceList string  `json:"top_domain_source_list"`
	}

	response := make([]anomalyResponse, 0, len(anomalies))
	for _, anomaly := range anomalies {
		response = append(response, anomalyResponse{
			DeviceMAC:           anomaly.DeviceMAC,
			DeviceName:          anomaly.DeviceName,
			Type:                anomaly.Type,
			CurrentValue:        anomaly.CurrentValue,
			AverageValue:        anomaly.AverageValue,
			Delta:               anomaly.Delta,
			TopDomain:           anomaly.TopDomain,
			TopDomainCategory:   anomaly.TopDomainCategory,
			TopDomainSourceList: anomaly.TopDomainSourceList,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAPIGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := loadRuntimeSettings(s.db)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_days":     settings.RetentionDays,
		"list_refresh_hours": settings.ListRefreshHours,
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
