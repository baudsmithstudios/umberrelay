package web

import (
	"log"
	"net/http"
	"strconv"

	"scrye/internal/store"
)

type pageData struct {
	Title  string
	Active string
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
	stats, err := s.db.DashboardSummary()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Stats interface{}
	}{
		pageData: pageData{Title: "Dashboard", Active: "dashboard"},
		Stats:    stats,
	}
	s.renderPage(w, "dashboard", data)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.db.ListDevicesWithStats()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := struct {
		pageData
		Devices []store.DeviceWithStats
	}{
		pageData: pageData{Title: "Devices", Active: "devices"},
		Devices:  devices,
	}
	s.renderPage(w, "devices", data)
}

func (s *Server) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	mac := r.PathValue("mac")
	dev, err := s.db.GetDevice(mac)
	if err != nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	topDomains, _ := s.db.DeviceTopDomains(mac, 20)

	data := struct {
		pageData
		Device     interface{}
		TopDomains interface{}
	}{
		pageData:   pageData{Title: dev.Hostname, Active: "devices"},
		Device:     dev,
		TopDomains: topDomains,
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
