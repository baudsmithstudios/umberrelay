package web

import (
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
	ClassificationClass string
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
	DomainPage       int
	DomainPageCount  int
	DomainPageStart  int
	DomainPageEnd    int
	DomainHasPrev    bool
	DomainHasNext    bool
	DomainPrevPage   int
	DomainNextPage   int
}

const detailDomainsPageSize = 20

type detailDomainPagination struct {
	Page      int
	PageCount int
	Offset    int
	Start     int
	End       int
	HasPrev   bool
	HasNext   bool
	PrevPage  int
	NextPage  int
}

func makeDetailDomainPagination(requestedPage, totalDomains int) detailDomainPagination {
	pageCount := 1
	if totalDomains > 0 {
		pageCount = (totalDomains + detailDomainsPageSize - 1) / detailDomainsPageSize
	}

	page := requestedPage
	if page < 1 {
		page = 1
	}
	if page > pageCount {
		page = pageCount
	}

	offset := (page - 1) * detailDomainsPageSize
	start := 0
	end := 0
	if totalDomains > 0 {
		start = offset + 1
		end = offset + detailDomainsPageSize
		if end > totalDomains {
			end = totalDomains
		}
	}

	return detailDomainPagination{
		Page:      page,
		PageCount: pageCount,
		Offset:    offset,
		Start:     start,
		End:       end,
		HasPrev:   page > 1,
		HasNext:   page < pageCount,
		PrevPage:  page - 1,
		NextPage:  page + 1,
	}
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

func listRefreshStatusView(status store.ListRefreshStatus) (string, string) {
	if status.LastAttemptAt.IsZero() {
		return "List refresh has not run yet.", "status-neutral"
	}
	if status.LastError != "" {
		return fmt.Sprintf("Last refresh failed at %s: %s", status.LastAttemptAt.Local().Format("2006-01-02 15:04:05"), status.LastError), "status-error"
	}
	return fmt.Sprintf("Last refresh succeeded at %s.", status.LastSuccessAt.Local().Format("2006-01-02 15:04:05")), "status-ok"
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
			Label:          source.Label,
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

func classificationPillClass(category string) string {
	return "classification-pill-" + groupCategory(category)
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
		ClassificationClass: classificationPillClass(domain.Category),
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

func findDomainRow(domains []store.DomainWithSource, domain string, totalActors int, actorKey string) privacyDomainRow {
	for _, item := range domains {
		if item.Domain == domain {
			return makePrivacyDomainRow(item, totalActors, actorKey)
		}
	}
	return privacyDomainRow{}
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

func makePrivacyDetail(actorKey string, totalActors int, summary store.DevicePrivacySummary, queryTrend, trackerTrend store.Trend, categoryCounts []store.CategoryCount, topDomains []store.DomainWithSource, pagination detailDomainPagination) privacyDetail {
	domains := make([]privacyDomainRow, 0, len(topDomains))
	for _, domain := range topDomains {
		domains = append(domains, makePrivacyDomainRow(domain, totalActors, actorKey))
	}

	return privacyDetail{
		LiveActorKey:     actorKey,
		CategoryOptions:  category.Options(),
		PrivacySummary:   summary,
		QueryTrend:       formatTrend(queryTrend, false),
		TrackerTrend:     formatTrend(trackerTrend, true),
		Breakdown:        makeBreakdownRows(summary.QueryCount, categoryCounts),
		Domains:          domains,
		RangeQuery:       "actor=" + url.QueryEscape(actorKey),
		ChartDescription: "Tracker rate and volume over time",
		EmptyMessage:     "No domains in the last 24 hours.",
		DomainPage:       pagination.Page,
		DomainPageCount:  pagination.PageCount,
		DomainPageStart:  pagination.Start,
		DomainPageEnd:    pagination.End,
		DomainHasPrev:    pagination.HasPrev,
		DomainHasNext:    pagination.HasNext,
		DomainPrevPage:   pagination.PrevPage,
		DomainNextPage:   pagination.NextPage,
	}
}

func (s *Server) loadDeviceDetail(now time.Time, device store.Device, totalActors int, domainsPage int) (privacyDetail, error) {
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
	pagination := makeDetailDomainPagination(domainsPage, privacySummary.UniqueDomains)
	topDomains, err := s.db.DeviceTopDomainsWithSourcePage(device.MAC, detailDomainsPageSize, pagination.Offset)
	if err != nil {
		return privacyDetail{}, err
	}

	detail := makePrivacyDetail(actorKeyForDevice(device.MAC), totalActors, privacySummary, queryTrend, trackerTrend, categoryCounts, topDomains, pagination)
	detail.Mode = "device"
	detail.Title = "Device Detail"
	detail.Subtitle = detailSubtitle(device)
	detail.Device = device
	detail.DeviceName = deviceDisplayName(device)
	detail.ChartID = "device-trend-chart"
	detail.ChartTitle = "Device Trend"
	return detail, nil
}

func (s *Server) loadSourceDetail(now time.Time, sourceIP string, totalActors int, domainsPage int) (privacyDetail, error) {
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
	pagination := makeDetailDomainPagination(domainsPage, privacySummary.UniqueDomains)
	topDomains, err := s.db.SourceTopDomainsWithSourcePage(sourceIP, detailDomainsPageSize, pagination.Offset)
	if err != nil {
		return privacyDetail{}, err
	}

	subtitle := "Unattributed source · " + sourceIP
	if label != "" {
		subtitle = sourceIP
	}

	detail := makePrivacyDetail(actorKeyForSource(sourceIP), totalActors, privacySummary, queryTrend, trackerTrend, categoryCounts, topDomains, pagination)
	detail.Mode = "source"
	detail.Title = "Source Detail"
	detail.Subtitle = subtitle
	detail.DeviceName = sourceActorDisplayName(sourceIP, label)
	detail.SourceIP = sourceIP
	detail.SourceLabel = label
	detail.ChartID = "source-trend-chart"
	detail.ChartTitle = "Source Trend"
	return detail, nil
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
	domainsPage := 1
	if pageValue := r.URL.Query().Get("page"); pageValue != "" {
		if parsedPage, err := parseBoundedInt(pageValue, 1, 1000000); err == nil {
			domainsPage = parsedPage
		}
	}
	selectedRaw := r.PathValue("mac")
	selectedType, selectedValue, hasSelected := normalizeActorSelection(selectedRaw)
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
		view.Detail, err = s.loadDeviceDetail(now, selected.Device, totalActors, domainsPage)
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
		view.Detail, err = s.loadSourceDetail(now, selected.SourceIP, totalActors, domainsPage)
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
	refreshStatus, err := s.db.GetListRefreshStatus()
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
		RetentionDays          int
		ListRefreshHours       int
		ListRefreshStatusClass string
		ListRefreshStatusText  string
		Lists                  interface{}
	}{
		pageData:         pageData{Title: "Settings", Active: "settings"},
		RetentionDays:    settings.RetentionDays,
		ListRefreshHours: settings.ListRefreshHours,
		Lists:            lists,
	}
	data.ListRefreshStatusText, data.ListRefreshStatusClass = listRefreshStatusView(refreshStatus)
	s.renderPage(w, "settings", data)
}

func (s *Server) handleUIUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !parseUIForm(w, r) {
		return
	}

	input, err := settingsInputFromForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := app.UpdateSettings(s.db, s.classify, input); err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) handleUIUpdateDeviceLabel(w http.ResponseWriter, r *http.Request) {
	if !parseUIForm(w, r) {
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
	if !parseUIForm(w, r) {
		return
	}

	sourceIP := r.PathValue("ip")
	if err := s.db.SetSourceLabel(sourceIP, r.FormValue("label")); err != nil {
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
	if !parseUIForm(w, r) {
		return
	}

	domain := r.PathValue("domain")
	categoryValue := r.FormValue("category")
	if categoryValue == "" {
		http.Error(w, "category is required", http.StatusBadRequest)
		return
	}
	normalizedCategory, ok := category.Normalize(categoryValue)
	if !ok {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}

	if err := app.SetDomainOverride(s.db, s.classify, domain, normalizedCategory); err != nil {
		if errors.Is(err, app.ErrInvalidCategory) {
			http.Error(w, "invalid category", http.StatusBadRequest)
			return
		}
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

	var domains []store.DomainWithSource
	rowActorKey := actorKey
	actorType, actorValue, hasActor := parseActorKey(actorKey)
	switch {
	case hasActor && actorType == actorTypeDevice:
		domains, err = s.db.DeviceTopDomainsWithSourcePage(actorValue, 20, 0)
	case hasActor && actorType == actorTypeSource:
		domains, err = s.db.SourceTopDomainsWithSourcePage(actorValue, 20, 0)
	default:
		domains, err = s.db.TopDomainsWithSource(20)
		rowActorKey = ""
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	row := findDomainRow(domains, domain, totalActors, rowActorKey)
	if row.Domain == "" {
		row = makePrivacyDomainRow(store.DomainWithSource{
			Domain:      domain,
			Category:    normalizedCategory,
			QueryCount:  0,
			DeviceCount: 0,
			SourceList:  "manual",
		}, totalActors, actorKey)
	}
	row.Category = normalizedCategory
	row.CategoryLabel = categoryLabel(normalizedCategory)
	row.SourceList = "manual"
	row.ClassificationLabel = fmt.Sprintf("%s · manual", row.CategoryLabel)
	row.ClassificationClass = classificationPillClass(normalizedCategory)

	s.renderFragment(w, "fragments", "domain-row", row)
}

func (s *Server) handleUIAddList(w http.ResponseWriter, r *http.Request) {
	if !parseUIForm(w, r) {
		return
	}

	_, err := app.AddList(r.Context(), s.db, app.AddListInput{
		URL:      r.FormValue("url"),
		Name:     r.FormValue("name"),
		Category: r.FormValue("category"),
	})
	if err != nil {
		if errors.Is(err, app.ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
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
	if !parseUIForm(w, r) {
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
	s.refreshClassificationAsync()
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func parseUIForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return false
	}
	return true
}

func settingsInputFromForm(r *http.Request) (app.SettingsInput, error) {
	retentionDays, err := parseBoundedInt(r.FormValue("retention_days"), app.RetentionDaysMin, app.RetentionDaysMax)
	if err != nil {
		return app.SettingsInput{}, fmt.Errorf("retention_days must be between %d and %d", app.RetentionDaysMin, app.RetentionDaysMax)
	}

	listRefreshHours, err := parseBoundedInt(r.FormValue("list_refresh_hours"), app.ListRefreshHoursMin, app.ListRefreshHoursMax)
	if err != nil {
		return app.SettingsInput{}, fmt.Errorf("list_refresh_hours must be between %d and %d", app.ListRefreshHoursMin, app.ListRefreshHoursMax)
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
