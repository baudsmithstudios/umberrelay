package web

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
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

type privacyDomainRow struct {
	Domain              string
	Category            string
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
	Mode            string
	LiveActorKey    string
	Device          store.Device
	DeviceName      string
	SourceIP        string
	SourceLabel     string
	PrivacySummary  store.DevicePrivacySummary
	ListMatches     listMatches
	TrackerChange   string
	Flag            *actorFlag
	IdentifiedBy    string
	Domains         []privacyDomainRow
	RangeQuery      string
	DomainPage      int
	DomainPageCount int
	DomainPageStart int
	DomainPageEnd   int
	DomainHasPrev   bool
	DomainHasNext   bool
	DomainPrevPage  int
	DomainNextPage  int
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

func listRefreshStatusView(status store.ListRefreshStatus) (string, string) {
	if status.LastAttemptAt.IsZero() {
		return "List refresh has not run yet.", ""
	}
	if status.LastError != "" {
		return fmt.Sprintf("Last refresh failed at %s: %s", status.LastAttemptAt.Local().Format("2006-01-02 15:04:05"), status.LastError), "error"
	}
	return fmt.Sprintf("Last refresh succeeded at %s.", status.LastSuccessAt.Local().Format("2006-01-02 15:04:05")), "success"
}

func trackerTrendText(t store.Trend) string {
	if !t.HasPrior {
		return ""
	}
	if math.Abs(t.Change) < 0.5 {
		return "0pp"
	}
	return fmt.Sprintf("%+dpp", int(math.Round(t.Change)))
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

func findDomainRow(domains []store.DomainWithSource, domain string, totalActors int, actorKey string) privacyDomainRow {
	for _, item := range domains {
		if item.Domain == domain {
			return makePrivacyDomainRow(item, totalActors, actorKey)
		}
	}
	return privacyDomainRow{}
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

func makePrivacyDetail(actorKey string, totalActors int, summary store.DevicePrivacySummary, matches listMatches, topDomains []store.DomainWithSource, pagination detailDomainPagination) privacyDetail {
	domains := make([]privacyDomainRow, 0, len(topDomains))
	for _, domain := range topDomains {
		domains = append(domains, makePrivacyDomainRow(domain, totalActors, actorKey))
	}

	return privacyDetail{
		LiveActorKey:    actorKey,
		PrivacySummary:  summary,
		ListMatches:     matches,
		Domains:         domains,
		RangeQuery:      "actor=" + url.QueryEscape(actorKey),
		DomainPage:      pagination.Page,
		DomainPageCount: pagination.PageCount,
		DomainPageStart: pagination.Start,
		DomainPageEnd:   pagination.End,
		DomainHasPrev:   pagination.HasPrev,
		DomainHasNext:   pagination.HasNext,
		DomainPrevPage:  pagination.PrevPage,
		DomainNextPage:  pagination.NextPage,
	}
}

func (s *Server) loadDeviceDetail(now time.Time, device store.Device, totalActors int, domainsPage int) (privacyDetail, error) {
	privacySummary, err := s.db.DevicePrivacySummaryAt(device.MAC, now)
	if err != nil {
		return privacyDetail{}, err
	}
	counts, err := s.db.DeviceCategoryCountsAt(device.MAC, now)
	if err != nil {
		return privacyDetail{}, err
	}
	pagination := makeDetailDomainPagination(domainsPage, privacySummary.UniqueDomains)
	topDomains, err := s.db.DeviceTopDomainsWithSourcePage(device.MAC, detailDomainsPageSize, pagination.Offset)
	if err != nil {
		return privacyDetail{}, err
	}

	detail := makePrivacyDetail(actorKeyForDevice(device.MAC), totalActors, privacySummary, makeListMatches(counts), topDomains, pagination)
	detail.Mode = actorTypeDevice
	detail.IdentifiedBy = identificationNote(device)
	detail.Device = device
	detail.DeviceName = deviceDisplayName(device)
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
	counts, err := s.db.SourceCategoryCountsAt(sourceIP, now)
	if err != nil {
		return privacyDetail{}, err
	}
	pagination := makeDetailDomainPagination(domainsPage, privacySummary.UniqueDomains)
	topDomains, err := s.db.SourceTopDomainsWithSourcePage(sourceIP, detailDomainsPageSize, pagination.Offset)
	if err != nil {
		return privacyDetail{}, err
	}

	detail := makePrivacyDetail(actorKeyForSource(sourceIP), totalActors, privacySummary, makeListMatches(counts), topDomains, pagination)
	detail.Mode = actorTypeSource
	detail.IdentifiedBy = "No MAC address was seen for this source, so it is tracked by IP."
	detail.DeviceName = sourceActorDisplayName(sourceIP, label)
	detail.SourceIP = sourceIP
	detail.SourceLabel = label
	return detail, nil
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
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
	stats, err := s.db.DashboardSummaryAt(now)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, networkTrend, err := s.db.LoadTrendsAt(now, "")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	totalActors := len(devices) + len(sources)
	flags := flagsByMAC(anomalies)

	view := activityView{pageData: pageData{Active: "activity"}, Tab: parseTab(r.URL.Query().Get("tab"))}
	selectedKey := ""
	if raw := r.PathValue("key"); raw != "" {
		actorType, value, ok := normalizeActorSelection(raw)
		if !ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		domainsPage := 1
		if pageValue := r.URL.Query().Get("page"); pageValue != "" {
			if parsedPage, err := parseBoundedInt(pageValue, 1, 1000000); err == nil {
				domainsPage = parsedPage
			}
		}
		var detail privacyDetail
		switch actorType {
		case actorTypeDevice:
			selected := findDevice(devices, value)
			if selected == nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			detail, err = s.loadDeviceDetail(now, selected.Device, totalActors, domainsPage)
			detail.TrackerChange = trackerTrendText(selected.TrackerTrend)
			if flag, ok := flags[selected.MAC]; ok {
				detail.Flag = &flag
			}
		default:
			selected := findSource(sources, value)
			if selected == nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			detail, err = s.loadSourceDetail(now, value, totalActors, domainsPage)
			detail.TrackerChange = trackerTrendText(selected.TrackerTrend)
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		selectedKey = detail.LiveActorKey
		view.Title = detail.DeviceName
		view.Detail = &detail
	} else {
		pane, err := s.loadNetworkPane(now, stats, anomalies, totalActors)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		pane.TrackerChange = trackerTrendText(networkTrend)
		view.Title = "Activity"
		view.Network = &pane
	}
	view.Roster = makeRoster(devices, sources, stats, flags, selectedKey)
	s.renderPage(w, "activity", view)
}

func (s *Server) loadNetworkPane(now time.Time, stats store.DashboardStats, anomalies []store.Anomaly, totalActors int) (networkPane, error) {
	counts, err := s.db.NetworkCategoryCountsAt(now)
	if err != nil {
		return networkPane{}, err
	}
	topDomains, err := s.db.TopDomainsWithSource(10)
	if err != nil {
		return networkPane{}, err
	}
	pane := networkPane{
		Stats:       stats,
		ListMatches: makeListMatches(counts),
	}
	for _, anomaly := range anomalies {
		pane.Flags = append(pane.Flags, flagFor(anomaly))
	}
	for _, domain := range topDomains {
		pane.TopDomains = append(pane.TopDomains, makePrivacyDomainRow(domain, totalActors, ""))
	}
	return pane, nil
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
		RetentionDays         int
		ListRefreshHours      int
		ListRefreshStatusTone string
		ListRefreshStatusText string
		Lists                 interface{}
	}{
		pageData:         pageData{Title: "Settings", Active: "settings"},
		RetentionDays:    settings.RetentionDays,
		ListRefreshHours: settings.ListRefreshHours,
		Lists:            lists,
	}
	data.ListRefreshStatusText, data.ListRefreshStatusTone = listRefreshStatusView(refreshStatus)
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

	if err := app.SetDomainOverride(s.classify, domain, normalizedCategory); err != nil {
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
