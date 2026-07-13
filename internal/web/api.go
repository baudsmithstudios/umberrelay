package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"umberrelay/internal/app"
	"umberrelay/internal/category"
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
			Name:           sourceActorDisplayName(source.SourceIP, source.Label),
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
		if errors.Is(err, store.ErrNotFound) {
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

func (s *Server) handleAPIQueryStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	actorKey := q.Get("actor")
	deviceMAC := q.Get("device")
	domain := q.Get("domain")
	categoryFilter := strings.TrimSpace(q.Get("category"))
	afterID := int64(0)
	limit := 100

	if v := q.Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSONError(w, http.StatusBadRequest, "after must be a non-negative integer")
			return
		}
		afterID = n
	} else if v := strings.TrimSpace(r.Header.Get("Last-Event-ID")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSONError(w, http.StatusBadRequest, "Last-Event-ID must be a non-negative integer")
			return
		}
		afterID = n
	}

	if v := q.Get("limit"); v != "" {
		n, err := parseBoundedInt(v, 1, 500)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		limit = n
	}

	filter := store.QueryFeedFilter{
		Domain: domain,
	}

	if categoryFilter != "" {
		normalizedCategory, ok := category.Normalize(categoryFilter)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "invalid category filter")
			return
		}
		filter.Category = normalizedCategory
	}

	if actorKey != "" {
		actorType, actorValue, ok := parseActorKey(actorKey)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "actor must be device:{mac} or source:{ip}")
			return
		}
		if actorType == actorTypeSource {
			filter.SourceIP = actorValue
		} else {
			filter.DeviceMAC = actorValue
		}
	} else if deviceMAC != "" {
		filter.DeviceMAC = deviceMAC
	}

	initial, err := s.db.QueryFeed(afterID, filter, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	lastID, err := writeQueryStreamBatch(w, initial)
	if err != nil {
		log.Printf("stream initial write failed: %v", err)
		return
	}
	if len(initial) > 0 {
		afterID = lastID
		flusher.Flush()
	}
	s.queryHub.AdvanceCursor(afterID)
	stream, cancel := s.queryHub.Subscribe()
	defer cancel()
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeatTicker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case query, ok := <-stream:
			if !ok {
				return
			}
			if query.ID <= afterID {
				continue
			}
			if !queryMatchesFeedFilter(query, filter) {
				continue
			}
			lastID, err = writeQueryStreamBatch(w, []store.Query{query})
			if err != nil {
				log.Printf("stream batch write failed: %v", err)
				return
			}
			afterID = lastID
			flusher.Flush()
		}
	}
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
	if anomalies == nil {
		anomalies = []store.Anomaly{}
	}
	writeJSON(w, http.StatusOK, anomalies)
}

func (s *Server) handleAPIBypass(w http.ResponseWriter, r *http.Request) {
	signals, err := s.db.DeviceBypassSignalsAt(time.Now())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	type bypassResponse struct {
		DeviceMAC       string `json:"device_mac"`
		DeviceName      string `json:"device_name"`
		Confidence      string `json:"confidence"`
		HintDomain      string `json:"hint_domain"`
		SilentMinutes   int    `json:"silent_minutes"`
		PriorQueryCount int    `json:"prior_query_count"`
		LastSeen        int64  `json:"last_seen"`
		LastQuery       int64  `json:"last_query"`
	}

	response := make([]bypassResponse, 0, len(signals))
	for _, signal := range signals {
		row := bypassResponse{
			DeviceMAC:       signal.DeviceMAC,
			DeviceName:      signal.DeviceName,
			Confidence:      signal.Confidence,
			HintDomain:      signal.HintDomain,
			SilentMinutes:   signal.SilentMinutes,
			PriorQueryCount: signal.PriorQueryCount,
			LastSeen:        signal.LastSeen.Unix(),
		}
		if !signal.LastQuery.IsZero() {
			row.LastQuery = signal.LastQuery.Unix()
		}
		response = append(response, row)
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

func (s *Server) handleAPIListRefreshStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.db.GetListRefreshStatus()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	response := struct {
		LastAttemptAt int64  `json:"last_attempt_at"`
		LastSuccessAt int64  `json:"last_success_at"`
		LastError     string `json:"last_error"`
	}{
		LastError: status.LastError,
	}
	if !status.LastAttemptAt.IsZero() {
		response.LastAttemptAt = status.LastAttemptAt.Unix()
	}
	if !status.LastSuccessAt.IsZero() {
		response.LastSuccessAt = status.LastSuccessAt.Unix()
	}
	writeJSON(w, http.StatusOK, response)
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
		if errors.Is(err, app.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
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
		if errors.Is(err, app.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal error")
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
	s.refreshClassificationAsync()
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
		if errors.Is(err, app.ErrInvalidCategory) {
			writeJSONError(w, http.StatusBadRequest, "invalid category")
			return
		}
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

func writeQueryStreamBatch(w http.ResponseWriter, queries []store.Query) (int64, error) {
	var lastID int64
	for _, query := range queries {
		payload, err := json.Marshal(struct {
			ID        int64  `json:"id"`
			ActorKey  string `json:"actor_key"`
			DeviceMAC string `json:"device_mac"`
			SourceIP  string `json:"source_ip"`
			Domain    string `json:"domain"`
			QueryType string `json:"query_type"`
			Category  string `json:"category"`
			Timestamp int64  `json:"timestamp"`
		}{
			ID:        query.ID,
			ActorKey:  streamActorKey(query),
			DeviceMAC: query.DeviceMAC,
			SourceIP:  query.SourceIP,
			Domain:    query.Domain,
			QueryType: query.QueryType,
			Category:  query.Category,
			Timestamp: query.Timestamp.Unix(),
		})
		if err != nil {
			return lastID, err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: query\ndata: %s\n\n", query.ID, payload); err != nil {
			return lastID, err
		}
		lastID = query.ID
	}
	return lastID, nil
}

func streamActorKey(query store.Query) string {
	if query.DeviceMAC != "" {
		return actorKeyForDevice(query.DeviceMAC)
	}
	return actorKeyForSource(query.SourceIP)
}

func queryMatchesFeedFilter(query store.Query, filter store.QueryFeedFilter) bool {
	if filter.SourceIP != "" {
		if query.DeviceMAC != "" || query.SourceIP != filter.SourceIP {
			return false
		}
	} else if filter.DeviceMAC != "" && query.DeviceMAC != filter.DeviceMAC {
		return false
	}

	if filter.Domain != "" && query.Domain != filter.Domain {
		return false
	}

	if filter.Category == category.Uncategorized {
		return query.Category == "" || query.Category == category.Uncategorized
	}
	if filter.Category != "" && query.Category != filter.Category {
		return false
	}

	return true
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
	if !s.refreshRunning.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.refreshRunning.Store(false)

		ctx, cancel := context.WithTimeout(s.backgroundCtx, 2*time.Minute)
		defer cancel()

		if ctx.Err() != nil {
			return
		}

		sources, err := s.loadEnabledListSources(s.db)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("load enabled list sources: %v", err)
			if recordErr := s.db.RecordListRefreshAttempt(time.Now().UTC(), err); recordErr != nil {
				if ctx.Err() == nil {
					log.Printf("record list refresh status: %v", recordErr)
				}
			}
			return
		}
		if err := s.refreshListSources(ctx, s.classify, sources); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("refresh list sources: %v", err)
		}
	}()
}
