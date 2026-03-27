package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"scrye/internal/classify"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleAPISummary(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.DashboardSummary()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleAPIDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.db.ListDevicesWithStats()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (s *Server) handleAPIDevice(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	dev, err := s.db.GetDevice(mac)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dev)
}

func (s *Server) handleAPIUpdateDevice(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	label, err := readDeviceLabel(r)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateDeviceLabel(mac, label); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
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
			http.Error(w, "limit must be between 1 and 1000", http.StatusBadRequest)
			return
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	var from, to time.Time
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			http.Error(w, "from must be RFC3339", http.StatusBadRequest)
			return
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			http.Error(w, "to must be RFC3339", http.StatusBadRequest)
			return
		}
		to = t
	}
	if to.IsZero() {
		to = time.Now()
	}
	if !from.IsZero() && to.Before(from) {
		http.Error(w, "to must be after from", http.StatusBadRequest)
		return
	}

	queries, err := s.db.QueryLog(deviceMAC, domain, from, to, limit, offset)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

func (s *Server) handleAPIDomains(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := parseBoundedInt(v, 1, 1000)
		if err != nil {
			http.Error(w, "limit must be between 1 and 1000", http.StatusBadRequest)
			return
		}
		limit = n
	}
	domains, err := s.db.TopDomains(limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(domains)
}

func (s *Server) handleAPIGetSettings(w http.ResponseWriter, r *http.Request) {
	retention, _ := s.db.GetConfig("retention_days")
	if retention == "" {
		retention = "30"
	}
	refreshHours, _ := s.db.GetConfig("list_refresh_hours")
	if refreshHours == "" {
		refreshHours = "24"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"retention_days":     retention,
		"list_refresh_hours": refreshHours,
	})
}

var validConfigKeys = map[string]bool{
	"retention_days":     true,
	"list_refresh_hours": true,
}

func (s *Server) handleAPIUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}
	for key := range r.PostForm {
		if !validConfigKeys[key] {
			http.Error(w, "unknown setting: "+key, http.StatusBadRequest)
			return
		}
	}
	for key, values := range r.PostForm {
		if len(values) > 0 {
			if err := validateSettingValue(key, values[0]); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := s.db.SetConfig(key, values[0]); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
	}
	if s.classify != nil {
		s.classify.NotifyConfigChanged()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIListLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.db.ListLists()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(lists)
}

func (s *Server) handleAPIAddList(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}
	url := r.FormValue("url")
	name := r.FormValue("name")
	category := r.FormValue("category")
	if url == "" || name == "" || category == "" {
		http.Error(w, "url, name, and category are required", http.StatusBadRequest)
		return
	}
	if !isValidCategory(category) {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	if _, err := classify.ParseAndValidateListURL(r.Context(), url); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := s.db.AddList(url, name, category)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

func (s *Server) handleAPIUpdateList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid list id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}
	enabled, err := parseBoolFormValue(r.FormValue("enabled"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateListEnabled(id, enabled); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeleteList(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid list id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteList(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIRefreshLists(w http.ResponseWriter, r *http.Request) {
	if s.classify == nil {
		http.Error(w, "classify manager not available", http.StatusServiceUnavailable)
		return
	}
	lists, err := s.db.ListLists()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var sources []classify.ListSource
	for _, l := range lists {
		if l.Enabled {
			sources = append(sources, classify.ListSource{
				ID:       l.ID,
				URL:      l.URL,
				Name:     l.Name,
				Category: l.Category,
			})
		}
	}
	go s.classify.Refresh(context.Background(), sources)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleAPISetOverride(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	var body struct {
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Category == "" {
		http.Error(w, "category is required", http.StatusBadRequest)
		return
	}
	if s.classify != nil {
		s.classify.SetOverride(domain, body.Category)
	} else {
		s.db.SetDomainOverride(domain, body.Category)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeleteOverride(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if s.classify != nil {
		s.classify.RemoveOverride(domain)
	} else {
		s.db.DeleteDomainOverride(domain)
	}
	w.WriteHeader(http.StatusNoContent)
}

func readDeviceLabel(r *http.Request) (string, error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Label string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return "", err
		}
		return body.Label, nil
	}
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	return r.FormValue("label"), nil
}

func parseBoundedInt(value string, min, max int) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("value must be between %d and %d", min, max)
	}
	return n, nil
}

func validateSettingValue(key, value string) error {
	switch key {
	case "retention_days":
		if _, err := parseBoundedInt(value, 1, 365); err != nil {
			return fmt.Errorf("retention_days must be between 1 and 365")
		}
	case "list_refresh_hours":
		if _, err := parseBoundedInt(value, 1, 168); err != nil {
			return fmt.Errorf("list_refresh_hours must be between 1 and 168")
		}
	}
	return nil
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

func isValidCategory(category string) bool {
	switch category {
	case "tracking", "advertising", "analytics", "telemetry", "malware", "uncategorized":
		return true
	default:
		return false
	}
}

func (s *Server) refreshClassificationAsync() {
	if s.classify == nil {
		return
	}
	lists, err := s.db.ListLists()
	if err != nil {
		return
	}
	var sources []classify.ListSource
	for _, l := range lists {
		if l.Enabled {
			sources = append(sources, classify.ListSource{
				ID:       l.ID,
				URL:      l.URL,
				Name:     l.Name,
				Category: l.Category,
			})
		}
	}
	go s.classify.Refresh(context.Background(), sources)
}
