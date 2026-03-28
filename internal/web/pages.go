package web

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"

	"scrye/internal/app"
	"scrye/internal/store"
)

type pageData struct {
	Title  string
	Active string
}

type categoryRow struct {
	Category string
	Count    int
	Percent  float64
}

type TrendDisplay struct {
	Text  string
	Class string
}

type deviceTrendRow struct {
	MAC            string
	Hostname       string
	Vendor         string
	Label          string
	IP             string
	QueryCount     int
	TrackerPercent float64
	QueryTrend     TrendDisplay
	TrackerTrend   TrendDisplay
}

func formatTrend(t store.Trend, isTrackerPct bool) TrendDisplay {
	if !t.HasPrior {
		return TrendDisplay{}
	}

	display := TrendDisplay{}
	switch {
	case math.Abs(t.Change) < 0.5:
		display.Class = "trend-flat"
	case t.Change > 0:
		display.Class = "trend-up"
	default:
		display.Class = "trend-down"
	}

	suffix := "%"
	if isTrackerPct {
		suffix = "pp"
	}
	display.Text = fmt.Sprintf("%+d%s", int(math.Round(t.Change)), suffix)
	if display.Class == "trend-flat" {
		display.Text = fmt.Sprintf("%d%s", int(math.Round(t.Change)), suffix)
	}
	return display
}

func makeDeviceTrendRows(devices []store.DeviceWithTrends) []deviceTrendRow {
	rows := make([]deviceTrendRow, 0, len(devices))
	for _, device := range devices {
		rows = append(rows, deviceTrendRow{
			MAC:            device.MAC,
			Hostname:       device.Hostname,
			Vendor:         device.Vendor,
			Label:          device.Label,
			IP:             device.IP,
			QueryCount:     device.QueryCount,
			TrackerPercent: device.TrackerPercent,
			QueryTrend:     formatTrend(device.QueryTrend, false),
			TrackerTrend:   formatTrend(device.TrackerTrend, true),
		})
	}
	return rows
}

func (s *Server) renderPage(w http.ResponseWriter, name string, data interface{}) {
	t, ok := s.pages[name]
	if !ok {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	now := s.now()

	stats, err := s.db.DashboardSummaryAt(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	queryTrend, trackerTrend, err := s.db.LoadTrendsAt(now, "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	devices, err := s.db.ListDevicesWithTrendsAt(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	topDevices := makeDeviceTrendRows(devices)
	if len(topDevices) > 5 {
		topDevices = topDevices[:5]
	}
	data := struct {
		pageData
		Stats        store.DashboardStats
		QueryTrend   TrendDisplay
		TrackerTrend TrendDisplay
		TopDevices   []deviceTrendRow
	}{
		pageData:     pageData{Title: "Dashboard", Active: "dashboard"},
		Stats:        stats,
		QueryTrend:   formatTrend(queryTrend, false),
		TrackerTrend: formatTrend(trackerTrend, true),
		TopDevices:   topDevices,
	}
	s.renderPage(w, "dashboard", data)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.db.ListDevicesWithTrends()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Devices []deviceTrendRow
	}{
		pageData: pageData{Title: "Devices", Active: "devices"},
		Devices:  makeDeviceTrendRows(devices),
	}
	s.renderPage(w, "devices", data)
}

func (s *Server) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	now := s.now()
	dev, err := s.db.GetDevice(mac)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	privacySummary, err := s.db.DevicePrivacySummaryAt(mac, now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	queryTrend, trackerTrend, err := s.db.LoadTrendsAt(now, "device_mac = ?", mac)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	categoryCounts, err := s.db.DeviceCategoryBreakdown(mac)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	topDomains, err := s.db.DeviceTopDomains(mac, 20)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	categoryBreakdown := make([]categoryRow, 0, len(categoryCounts))
	for _, count := range categoryCounts {
		row := categoryRow{
			Category: count.Category,
			Count:    count.Count,
		}
		if privacySummary.QueryCount > 0 {
			row.Percent = float64(count.Count) / float64(privacySummary.QueryCount) * 100
		}
		categoryBreakdown = append(categoryBreakdown, row)
	}

	data := struct {
		pageData
		Device            store.Device
		PrivacySummary    store.DevicePrivacySummary
		QueryTrend        TrendDisplay
		TrackerTrend      TrendDisplay
		CategoryBreakdown []categoryRow
		TopDomains        []store.DeviceDomainSummary
	}{
		pageData:          pageData{Title: dev.Hostname, Active: "devices"},
		Device:            dev,
		PrivacySummary:    privacySummary,
		QueryTrend:        formatTrend(queryTrend, false),
		TrackerTrend:      formatTrend(trackerTrend, true),
		CategoryBreakdown: categoryBreakdown,
		TopDomains:        topDomains,
	}
	s.renderPage(w, "device", data)
}

func (s *Server) handleDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := s.db.TopDomains(100)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Domains interface{}
	}{
		pageData: pageData{Title: "Domains", Active: "domains"},
		Domains:  domains,
	}
	s.renderPage(w, "domains", data)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	retentionStr, _ := s.db.GetConfig("retention_days")
	retention := 30
	if retentionStr != "" {
		if n, err := strconv.Atoi(retentionStr); err == nil {
			retention = n
		}
	}

	refreshStr, _ := s.db.GetConfig("list_refresh_hours")
	refreshHours := 24
	if refreshStr != "" {
		if n, err := strconv.Atoi(refreshStr); err == nil {
			refreshHours = n
		}
	}

	lists, _ := s.db.ListLists()

	data := struct {
		pageData
		RetentionDays    int
		ListRefreshHours int
		Lists            interface{}
	}{
		pageData:         pageData{Title: "Settings", Active: "settings"},
		RetentionDays:    retention,
		ListRefreshHours: refreshHours,
		Lists:            lists,
	}
	s.renderPage(w, "settings", data)
}

func (s *Server) handleUIUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	input, err := settingsInputFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := app.UpdateSettings(s.db, s.classify, input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUIUpdateDeviceLabel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	mac := r.PathValue("mac")
	if err := app.UpdateDeviceLabel(s.db, mac, r.FormValue("label")); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) handleUIAddList(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	_, err := app.AddList(context.Background(), s.db, app.AddListInput{
		URL:      r.FormValue("url"),
		Name:     r.FormValue("name"),
		Category: r.FormValue("category"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.refreshClassificationAsync()
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUIUpdateList(w http.ResponseWriter, r *http.Request) {
	id, err := parseListID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	if err := app.UpdateListEnabled(s.db, id, enabled); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUIDeleteList(w http.ResponseWriter, r *http.Request) {
	id, err := parseListID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := app.DeleteList(s.db, id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func settingsInputFromForm(r *http.Request) (app.SettingsInput, error) {
	retentionDays, err := parseBoundedInt(r.FormValue("retention_days"), 1, 365)
	if err != nil {
		return app.SettingsInput{}, fmt.Errorf("retention_days must be between 1 and 365")
	}

	listRefreshHours, err := parseBoundedInt(r.FormValue("list_refresh_hours"), 1, 168)
	if err != nil {
		return app.SettingsInput{}, fmt.Errorf("list_refresh_hours must be between 1 and 168")
	}

	return app.SettingsInput{
		RetentionDays:    &retentionDays,
		ListRefreshHours: &listRefreshHours,
	}, nil
}

func parseListID(idStr string) (int64, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid list id")
	}
	return id, nil
}
