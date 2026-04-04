package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"umberrelay/internal/app"
	"umberrelay/internal/category"
	"umberrelay/internal/store"
)

type pageData struct {
	Title  string
	Active string
}

type categoryRow struct {
	Key     string
	Label   string
	Count   int
	Percent float64
	Class   string
}

type TrendDisplay struct {
	Text  string
	Class string
}

type deviceTrendRow struct {
	ActorKey       string
	ActorType      string
	MAC            string
	SourceIP       string
	Hostname       string
	Vendor         string
	Label          string
	IP             string
	QueryCount     int
	TrackerPercent float64
	QueryTrend     TrendDisplay
	TrackerTrend   TrendDisplay
	AnomalyClass   string
}

type anomalyRow struct {
	DeviceMAC   string
	DeviceName  string
	Badge       string
	Class       string
	Explanation string
}

type privacyDomainRow struct {
	Domain              string
	Category            string
	CategoryOptions     []category.Option
	CategoryLabel       string
	SourceList          string
	QueryCount          int
	DeviceCount         int
	DeviceCountLabel    string
	ClassificationLabel string
	ActorKey            string
	Scope               string
}

type privacyDetail struct {
	Mode             string
	Title            string
	Subtitle         string
	LiveActorKey     string
	CategoryOptions  []category.Option
	Device           store.Device
	DeviceName       string
	SourceIP         string
	SourceLabel      string
	PrivacySummary   store.DevicePrivacySummary
	QueryTrend       TrendDisplay
	TrackerTrend     TrendDisplay
	Breakdown        []categoryRow
	Domains          []privacyDomainRow
	RangeQuery       string
	ChartID          string
	ChartTitle       string
	ChartDescription string
	EmptyMessage     string
}

type homePageView struct {
	pageData
	Stats             store.DashboardStats
	TrackerRate       float64
	OverviewBreakdown []categoryRow
	Anomalies         []anomalyRow
	TopDomains        []privacyDomainRow
	TotalActors       int
}

type devicesListView struct {
	pageData
	Devices     []deviceTrendRow
	TotalActors int
}

type deviceDetailView struct {
	pageData
	Detail             privacyDetail
	SelectedDeviceName string
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

func makeDeviceTrendRows(devices []store.DeviceWithTrends, sources []store.SourceWithTrends) []deviceTrendRow {
	rows := make([]deviceTrendRow, 0, len(devices)+len(sources))
	for _, device := range devices {
		rows = append(rows, deviceTrendRow{
			ActorKey:       actorKeyForDevice(device.MAC),
			ActorType:      actorTypeDevice,
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
	for _, source := range sources {
		rows = append(rows, deviceTrendRow{
			ActorKey:       actorKeyForSource(source.SourceIP),
			ActorType:      actorTypeSource,
			SourceIP:       source.SourceIP,
			Label:          sourceActorDisplayName(source.SourceIP, ""),
			IP:             source.SourceIP,
			QueryCount:     source.QueryCount,
			TrackerPercent: source.TrackerPercent,
			QueryTrend:     formatTrend(source.QueryTrend, false),
			TrackerTrend:   formatTrend(source.TrackerTrend, true),
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

func (s *Server) renderFragment(w http.ResponseWriter, templateName, blockName string, data interface{}) {
	t, ok := s.pages[templateName]
	if !ok {
		http.Error(w, "page not found", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, blockName, data); err != nil {
		log.Printf("render %s/%s: %v", templateName, blockName, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}

func isHXRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

func deviceDisplayName(device store.Device) string {
	switch {
	case device.Label != "":
		return device.Label
	case device.Hostname != "":
		return device.Hostname
	case device.Vendor != "":
		return device.Vendor
	default:
		return device.MAC
	}
}

func deviceTrendDisplayName(device deviceTrendRow) string {
	if device.ActorType == actorTypeSource {
		return sourceActorDisplayName(device.SourceIP, "")
	}
	switch {
	case device.Label != "":
		return device.Label
	case device.Hostname != "":
		return device.Hostname
	case device.Vendor != "":
		return device.Vendor
	default:
		return device.MAC
	}
}

func detailSubtitle(device store.Device) string {
	parts := make([]string, 0, 4)
	if device.Vendor != "" {
		parts = append(parts, device.Vendor)
	}
	if device.IP != "" {
		parts = append(parts, device.IP)
	}
	if device.MAC != "" {
		parts = append(parts, device.MAC)
	}
	if !device.FirstSeen.IsZero() {
		parts = append(parts, "First seen "+device.FirstSeen.Local().Format("2006-01-02"))
	}
	return strings.Join(parts, " · ")
}

func groupCategory(category string) string {
	switch category {
	case "tracking", "advertising", "malware":
		return "tracking"
	case "analytics":
		return "analytics"
	default:
		return "unclassified"
	}
}

func breakdownRowMeta(key string) (string, string) {
	switch key {
	case "tracking":
		return "Tracking", "breakdown-tracking"
	case "analytics":
		return "Analytics", "breakdown-analytics"
	default:
		return "Unclassified", "breakdown-unclassified"
	}
}

func makeBreakdownRows(total int, counts []store.CategoryCount) []categoryRow {
	grouped := map[string]int{
		"tracking":     0,
		"analytics":    0,
		"unclassified": 0,
	}
	for _, count := range counts {
		grouped[groupCategory(count.Category)] += count.Count
	}

	rows := make([]categoryRow, 0, 3)
	for _, key := range []string{"tracking", "analytics", "unclassified"} {
		label, className := breakdownRowMeta(key)
		row := categoryRow{
			Key:   key,
			Label: label,
			Count: grouped[key],
			Class: className,
		}
		if total > 0 {
			row.Percent = float64(row.Count) / float64(total) * 100
		}
		rows = append(rows, row)
	}
	return rows
}

func categoryLabel(category string) string {
	if category == "" {
		return "unclassified"
	}
	return category
}

func classificationLabel(domain store.DomainWithSource) string {
	return fmt.Sprintf("%s · %s", categoryLabel(domain.Category), domain.SourceList)
}

func makePrivacyDomainRow(domain store.DomainWithSource, totalActors int, actorKey string) privacyDomainRow {
	row := privacyDomainRow{
		Domain:              domain.Domain,
		Category:            domain.Category,
		CategoryOptions:     category.Options(),
		CategoryLabel:       categoryLabel(domain.Category),
		SourceList:          domain.SourceList,
		QueryCount:          domain.QueryCount,
		DeviceCount:         domain.DeviceCount,
		ClassificationLabel: classificationLabel(domain),
		ActorKey:            actorKey,
	}
	if actorKey == "" {
		row.Scope = "network"
		row.DeviceCountLabel = fmt.Sprintf("%d of %d", domain.DeviceCount, totalActors)
		return row
	}
	row.Scope = "actor"
	row.DeviceCountLabel = "This actor"
	return row
}

func anomalyClass(anomalyType string) string {
	switch anomalyType {
	case "tracker_spike":
		return "anomaly-tracker"
	case "dns_bypass_likely", "dns_bypass_suspected":
		return "anomaly-bypass"
	default:
		return "anomaly-volume"
	}
}

func anomalyBadge(anomaly store.Anomaly) string {
	switch anomaly.Type {
	case "tracker_spike":
		return fmt.Sprintf("+%.0fpp", anomaly.Delta)
	case "dns_bypass_likely":
		return "Likely"
	case "dns_bypass_suspected":
		return "Suspected"
	default:
		if anomaly.AverageValue <= 0 {
			return fmt.Sprintf("+%.0f", anomaly.Delta)
		}
		return fmt.Sprintf("%.1fx", anomaly.CurrentValue/anomaly.AverageValue)
	}
}

func anomalyExplanation(anomaly store.Anomaly) string {
	switch anomaly.Type {
	case "tracker_spike":
		return fmt.Sprintf("%s jumped above its usual tracker rate, led by %s from %s.", anomaly.DeviceName, anomaly.TopDomain, anomaly.TopDomainSourceList)
	case "dns_bypass_likely":
		return fmt.Sprintf("%s is present on the LAN but has not used local DNS for %d minutes. Earlier queries included %s, which suggests encrypted DNS usage.", anomaly.DeviceName, int(anomaly.Delta), anomaly.TopDomain)
	case "dns_bypass_suspected":
		return fmt.Sprintf("%s is present on the LAN but has not used local DNS for %d minutes despite prior DNS activity, so visibility may be bypassed.", anomaly.DeviceName, int(anomaly.Delta))
	default:
		return fmt.Sprintf("%s is making far more requests than usual, with %s contributing the largest spike from %s.", anomaly.DeviceName, anomaly.TopDomain, anomaly.TopDomainSourceList)
	}
}

func makeAnomalyRows(anomalies []store.Anomaly) []anomalyRow {
	rows := make([]anomalyRow, 0, len(anomalies))
	for _, anomaly := range anomalies {
		rows = append(rows, anomalyRow{
			DeviceMAC:   anomaly.DeviceMAC,
			DeviceName:  anomaly.DeviceName,
			Badge:       anomalyBadge(anomaly),
			Class:       anomalyClass(anomaly.Type),
			Explanation: anomalyExplanation(anomaly),
		})
	}
	return rows
}

func anomalyPriority(className string) int {
	switch className {
	case "anomaly-tracker":
		return 3
	case "anomaly-bypass":
		return 2
	default:
		return 1
	}
}

func selectedDeviceAnomalies(anomalies []store.Anomaly) map[string]string {
	out := make(map[string]string, len(anomalies))
	for _, anomaly := range anomalies {
		className := anomalyClass(anomaly.Type)
		existing, ok := out[anomaly.DeviceMAC]
		if ok && anomalyPriority(existing) >= anomalyPriority(className) {
			continue
		}
		out[anomaly.DeviceMAC] = className
	}
	return out
}

func bypassSignalAsAnomaly(signal store.BypassSignal) store.Anomaly {
	anomalyType := "dns_bypass_suspected"
	if signal.Confidence == "likely" {
		anomalyType = "dns_bypass_likely"
	}
	return store.Anomaly{
		DeviceMAC:    signal.DeviceMAC,
		DeviceName:   signal.DeviceName,
		Type:         anomalyType,
		CurrentValue: float64(signal.SilentMinutes),
		Delta:        float64(signal.SilentMinutes),
		TopDomain:    signal.HintDomain,
	}
}

func (s *Server) loadActorsWithTrends(now time.Time) ([]store.DeviceWithTrends, []store.SourceWithTrends, error) {
	devices, err := s.db.ListDevicesWithTrendsAt(now)
	if err != nil {
		return nil, nil, err
	}
	sources, err := s.db.ListSourceWithTrendsAt(now)
	if err != nil {
		return nil, nil, err
	}
	return devices, sources, nil
}

func (s *Server) loadAttentionAnomalies(now time.Time) ([]store.Anomaly, error) {
	anomalies, err := s.db.DeviceAnomalies()
	if err != nil {
		return nil, err
	}
	bypassSignals, err := s.db.DeviceBypassSignalsAt(now)
	if err != nil {
		return nil, err
	}
	for _, signal := range bypassSignals {
		anomalies = append(anomalies, bypassSignalAsAnomaly(signal))
	}
	return anomalies, nil
}

func findDevice(devices []store.DeviceWithTrends, mac string) *store.DeviceWithTrends {
	for i := range devices {
		if devices[i].MAC == mac {
			return &devices[i]
		}
	}
	return nil
}

func findSource(sources []store.SourceWithTrends, sourceIP string) *store.SourceWithTrends {
	for i := range sources {
		if sources[i].SourceIP == sourceIP {
			return &sources[i]
		}
	}
	return nil
}

func (s *Server) loadDeviceDetail(now time.Time, device store.Device, totalActors int) (privacyDetail, error) {
	privacySummary, err := s.db.DevicePrivacySummaryAt(device.MAC, now)
	if err != nil {
		return privacyDetail{}, err
	}
	queryTrend, trackerTrend, err := s.db.LoadTrendsAt(now, "device_mac = ?", device.MAC)
	if err != nil {
		return privacyDetail{}, err
	}
	categoryCounts, err := s.db.DeviceCategoryBreakdown(device.MAC)
	if err != nil {
		return privacyDetail{}, err
	}
	topDomains, err := s.db.DeviceTopDomainsWithSource(device.MAC, 20)
	if err != nil {
		return privacyDetail{}, err
	}

	domains := make([]privacyDomainRow, 0, len(topDomains))
	for _, domain := range topDomains {
		domains = append(domains, makePrivacyDomainRow(domain, totalActors, actorKeyForDevice(device.MAC)))
	}

	return privacyDetail{
		Mode:             "device",
		Title:            "Device Detail",
		Subtitle:         detailSubtitle(device),
		LiveActorKey:     actorKeyForDevice(device.MAC),
		CategoryOptions:  category.Options(),
		Device:           device,
		DeviceName:       deviceDisplayName(device),
		PrivacySummary:   privacySummary,
		QueryTrend:       formatTrend(queryTrend, false),
		TrackerTrend:     formatTrend(trackerTrend, true),
		Breakdown:        makeBreakdownRows(privacySummary.QueryCount, categoryCounts),
		Domains:          domains,
		RangeQuery:       "actor=" + url.QueryEscape(actorKeyForDevice(device.MAC)),
		ChartID:          "device-trend-chart",
		ChartTitle:       "Device Trend",
		ChartDescription: "Tracker rate and volume over time",
		EmptyMessage:     "No domains in the last 24 hours.",
	}, nil
}

func (s *Server) loadSourceDetail(now time.Time, sourceIP string, totalActors int) (privacyDetail, error) {
	privacySummary, err := s.db.SourcePrivacySummaryAt(sourceIP, now)
	if err != nil {
		return privacyDetail{}, err
	}
	label, err := s.db.GetSourceLabel(sourceIP)
	if err != nil {
		return privacyDetail{}, err
	}
	queryTrend, trackerTrend, err := s.db.LoadTrendsAt(now, "device_mac = '' AND source_ip = ?", sourceIP)
	if err != nil {
		return privacyDetail{}, err
	}
	categoryCounts, err := s.db.SourceCategoryBreakdown(sourceIP)
	if err != nil {
		return privacyDetail{}, err
	}
	topDomains, err := s.db.SourceTopDomainsWithSource(sourceIP, 20)
	if err != nil {
		return privacyDetail{}, err
	}

	domains := make([]privacyDomainRow, 0, len(topDomains))
	for _, domain := range topDomains {
		domains = append(domains, makePrivacyDomainRow(domain, totalActors, actorKeyForSource(sourceIP)))
	}

	deviceName := sourceActorDisplayName(sourceIP, label)
	subtitle := "Unattributed source · " + sourceIP
	if label != "" {
		subtitle = sourceIP
	}

	return privacyDetail{
		Mode:             "source",
		Title:            "Source Detail",
		Subtitle:         subtitle,
		LiveActorKey:     actorKeyForSource(sourceIP),
		CategoryOptions:  category.Options(),
		DeviceName:       deviceName,
		SourceIP:         sourceIP,
		SourceLabel:      label,
		PrivacySummary:   privacySummary,
		QueryTrend:       formatTrend(queryTrend, false),
		TrackerTrend:     formatTrend(trackerTrend, true),
		Breakdown:        makeBreakdownRows(privacySummary.QueryCount, categoryCounts),
		Domains:          domains,
		RangeQuery:       "actor=" + url.QueryEscape(actorKeyForSource(sourceIP)),
		ChartID:          "source-trend-chart",
		ChartTitle:       "Source Trend",
		ChartDescription: "Tracker rate and volume over time",
		EmptyMessage:     "No domains in the last 24 hours.",
	}, nil
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	stats, err := s.db.DashboardSummaryAt(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	breakdownCounts, err := s.db.NetworkCategoryBreakdown()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	anomalies, err := s.loadAttentionAnomalies(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	topDomains, err := s.db.TopDomainsWithSource(10)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	devices, sources, err := s.loadActorsWithTrends(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	totalActors := len(devices) + len(sources)

	domainRows := make([]privacyDomainRow, 0, len(topDomains))
	for _, domain := range topDomains {
		domainRows = append(domainRows, makePrivacyDomainRow(domain, totalActors, ""))
	}

	view := homePageView{
		pageData: pageData{
			Title:  "Home",
			Active: "home",
		},
		Stats:             stats,
		TrackerRate:       stats.TrackerPercent,
		OverviewBreakdown: makeBreakdownRows(stats.TotalQueries, breakdownCounts),
		Anomalies:         makeAnomalyRows(anomalies),
		TopDomains:        domainRows,
		TotalActors:       totalActors,
	}

	s.renderPage(w, "home", view)
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	devices, sources, err := s.loadActorsWithTrends(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	anomalies, err := s.loadAttentionAnomalies(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows := makeDeviceTrendRows(devices, sources)
	flags := selectedDeviceAnomalies(anomalies)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TrackerPercent == rows[j].TrackerPercent {
			return deviceTrendDisplayName(rows[i]) < deviceTrendDisplayName(rows[j])
		}
		return rows[i].TrackerPercent > rows[j].TrackerPercent
	})
	for i := range rows {
		rows[i].AnomalyClass = flags[rows[i].MAC]
	}

	view := devicesListView{
		pageData: pageData{
			Title:  "Devices",
			Active: "devices",
		},
		Devices:     rows,
		TotalActors: len(rows),
	}
	s.renderPage(w, "devices", view)
}

func (s *Server) handleDeviceDetail(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	selectedRaw := r.PathValue("mac")
	_, selectedType, selectedValue, hasSelected := normalizeActorSelection(selectedRaw)
	if !hasSelected {
		http.Redirect(w, r, "/devices", http.StatusSeeOther)
		return
	}

	devices, sources, err := s.loadActorsWithTrends(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	totalActors := len(devices) + len(sources)

	view := deviceDetailView{
		pageData: pageData{
			Active: "devices",
		},
	}

	switch selectedType {
	case actorTypeDevice:
		selected := findDevice(devices, selectedValue)
		if selected == nil {
			http.Redirect(w, r, "/devices", http.StatusSeeOther)
			return
		}
		view.Detail, err = s.loadDeviceDetail(now, selected.Device, totalActors)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		view.SelectedDeviceName = deviceDisplayName(selected.Device)
		view.pageData.Title = view.SelectedDeviceName
	case actorTypeSource:
		selected := findSource(sources, selectedValue)
		if selected == nil {
			http.Redirect(w, r, "/devices", http.StatusSeeOther)
			return
		}
		view.Detail, err = s.loadSourceDetail(now, selected.SourceIP, totalActors)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		view.SelectedDeviceName = view.Detail.DeviceName
		view.pageData.Title = view.SelectedDeviceName
	default:
		http.Redirect(w, r, "/devices", http.StatusSeeOther)
		return
	}

	if isHXRequest(r) {
		s.renderFragment(w, "device_detail", "detail-content", view)
		return
	}

	s.renderPage(w, "device_detail", view)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := loadRuntimeSettings(s.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	lists, err := s.db.ListLists()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := struct {
		pageData
		RetentionDays    int
		ListRefreshHours int
		Lists            interface{}
	}{
		pageData:         pageData{Title: "Settings", Active: "settings"},
		RetentionDays:    settings.RetentionDays,
		ListRefreshHours: settings.ListRefreshHours,
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
		if errors.Is(err, app.ErrDeviceNotFound) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if isHXRequest(r) {
		device, err := s.db.GetDevice(mac)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.renderFragment(w, "fragments", "label-edit", struct {
			Device     store.Device
			DeviceName string
		}{
			Device:     device,
			DeviceName: deviceDisplayName(device),
		})
		return
	}

	http.Redirect(w, r, "/devices/"+mac, http.StatusSeeOther)
}

func (s *Server) handleUIUpdateSourceLabel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	sourceIP := r.PathValue("ip")
	if err := app.UpdateSourceLabel(s.db, sourceIP, r.FormValue("label")); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if isHXRequest(r) {
		label, err := s.db.GetSourceLabel(sourceIP)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		s.renderFragment(w, "fragments", "source-label-edit", struct {
			DeviceName  string
			SourceIP    string
			SourceLabel string
		}{
			DeviceName:  sourceActorDisplayName(sourceIP, label),
			SourceIP:    sourceIP,
			SourceLabel: label,
		})
		return
	}

	http.Redirect(w, r, "/devices/"+actorKeyForSource(sourceIP), http.StatusSeeOther)
}

func (s *Server) handleUISetOverride(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	domain := r.PathValue("domain")
	category := r.FormValue("category")
	if category == "" {
		http.Error(w, "category is required", http.StatusBadRequest)
		return
	}

	if err := app.SetDomainOverride(s.db, s.classify, domain, category); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	actorKey := r.FormValue("actor_key")
	if actorKey == "" {
		if mac := r.FormValue("device_mac"); mac != "" {
			actorKey = actorKeyForDevice(mac)
		}
	}
	stats, err := s.db.DashboardSummary()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	totalActors := stats.DeviceCount

	var row privacyDomainRow
	actorType, actorValue, hasActor := parseActorKey(actorKey)
	if hasActor && actorType == actorTypeDevice {
		domains, err := s.db.DeviceTopDomainsWithSource(actorValue, 20)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, item := range domains {
			if item.Domain == domain {
				row = makePrivacyDomainRow(item, totalActors, actorKey)
				break
			}
		}
	} else if hasActor && actorType == actorTypeSource {
		domains, err := s.db.SourceTopDomainsWithSource(actorValue, 20)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, item := range domains {
			if item.Domain == domain {
				row = makePrivacyDomainRow(item, totalActors, actorKey)
				break
			}
		}
	} else {
		domains, err := s.db.TopDomainsWithSource(20)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, item := range domains {
			if item.Domain == domain {
				row = makePrivacyDomainRow(item, totalActors, "")
				break
			}
		}
	}
	if row.Domain == "" {
		row = makePrivacyDomainRow(store.DomainWithSource{
			Domain:      domain,
			Category:    category,
			QueryCount:  0,
			DeviceCount: 0,
			SourceList:  "manual",
		}, totalActors, actorKey)
	}
	row.Category = category
	row.CategoryLabel = categoryLabel(category)
	row.SourceList = "manual"
	row.ClassificationLabel = fmt.Sprintf("%s · manual", row.CategoryLabel)

	s.renderFragment(w, "fragments", "domain-row", row)
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
		if errors.Is(err, app.ErrListNotFound) {
			http.Error(w, "list not found", http.StatusNotFound)
			return
		}
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
		if errors.Is(err, app.ErrListNotFound) {
			http.Error(w, "list not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.refreshClassificationAsync()
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUIRefreshLists(w http.ResponseWriter, r *http.Request) {
	if s.classify == nil {
		http.Error(w, "classify manager not available", http.StatusServiceUnavailable)
		return
	}

	sources, err := app.EnabledListSources(s.db)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	go app.RefreshListSources(context.Background(), s.classify, sources)
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
