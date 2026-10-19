# Roster UI on ElumKit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Pico-based Home, Devices and Device Detail pages with one ElumKit roster layout (an actor list beside a detail pane), and replace the three-tone breakdown bar with a tracker-rate meter and a list-matches table.

**Architecture:** `/` and `/devices/{actorKey}` both render one `activity` template. It holds a server-rendered roster plus a pane for the network or the selected actor, with Overview, Domains and Live tabs chosen by `?tab=`. One new store query provides per-category counts for the current 24h and the prior 7 days. ElumKit 0.4.0 CSS source is copied into the embedded static tree and themed by token overrides in one app stylesheet.

**Tech Stack:** Go 1.26 `net/http` + `html/template`, `go:embed`, SQLite (modernc), HTMX, vanilla JS, ElumKit 0.4.0 (copied CSS source plus `tabs.js`).

**Spec:** `docs/superpowers/specs/2026-10-19-elumkit-roster-ui-design.md`. Read it before starting any task. Visual reference: artboards `Roster.dc.html` and `RosterDevice.dc.html` on https://claude.ai/artifact/7AQGfp9tume4Y7FkAQsD5v. The spec wins where the two disagree.

## Global Constraints

- ElumKit version **0.4.0**. Copy `packages/core-css/src/` (keeping its structure) and `packages/core-js/src/tabs.js`. Never use `dist/elumkit.css`.
- Copied ElumKit lives at `internal/web/static/elumkit/`. **Not** under any directory named `vendor`: `.gitignore` ignores `vendor/`.
- Theme only through `--elum-*` token overrides on `:root`, with the values in the spec's Palette table. App CSS covers only what ElumKit lacks (mini-meter, reference tick, page layout).
- Never encode state by color alone. Every tone sits on an `elum-status-label` or `elum-badge` that carries text.
- No chart with two y-scales.
- `/ui/*` form routes and field names stay unchanged. `mutationOriginGuard` stays.
- `GET /devices` returns 404. Unknown or malformed actor keys redirect to `/`.
- Category tones: tracking and malware → `error`; advertising → `warn`; analytics and telemetry → `success`.
- List-match order: `tracking, advertising, malware, analytics, telemetry`. Zero-count rows are omitted. `uncategorized` and `""` count as "matched no list".
- "Prior" always means the 7 days before the current 24h (`trendWindowAt`). Badge copy: `vs prior 7d`.
- TDD for every Go change. Tests use real SQLite via `testServer(t)` / `testDB(t)`. No mocks.
- Never use `git add -A` without running `git status` first. Never skip hooks. Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Empty database (no queries in 24h).** The network pane renders `0%`, the matches area shows its `elum-empty`, and no `NaN` or `Inf` appears anywhere. Test owned by Task 5.
2. **No prior-window data.** Delta cells read `─` and the tracker badge is omitted, never `+Inf` or `NaN`. Test owned by Task 2.
3. **IPv6 source actors** (`/devices/source:fe80::1`). The route resolves, the roster link round-trips, and the row is selected. Test owned by Task 4.
4. **Unknown `?tab=` value.** It falls back to Overview. Test owned by Task 5.
5. **`?tab=domains&page=999`.** It clamps to the last page (as `makeDetailDomainPagination` does) and stays on the Domains tab. Test owned by Task 6.

---

### Task 1: Store — per-category counts for both windows

**Files:**
- Modify: `internal/store/db.go` (replace `categoryBreakdown`, `DeviceCategoryBreakdown`, `SourceCategoryBreakdown` and `NetworkCategoryBreakdown`, near lines 960–1000 and 1281–1320)
- Test: `internal/store/db_test.go` (replace `TestDeviceCategoryBreakdown` and `TestNetworkCategoryBreakdown`)

**Interfaces:**
- Produces:
  ```go
  type CategoryWindowCount struct {
      Category string // raw stored value; "" for unclassified rows
      Current  int    // queries in [now-24h, now)
      Prior    int    // queries in [now-8d, now-24h)
  }
  func (d *DB) NetworkCategoryCountsAt(now time.Time) ([]CategoryWindowCount, error)
  func (d *DB) DeviceCategoryCountsAt(mac string, now time.Time) ([]CategoryWindowCount, error)
  func (d *DB) SourceCategoryCountsAt(sourceIP string, now time.Time) ([]CategoryWindowCount, error)
  ```
  All three return one entry per distinct category with a non-zero count in either window, sorted by category name. They share one private `categoryCountsAt(actor actorFilter, now time.Time)`, with windows from `trendWindowAt(now)`.
- Add the new functions alongside the old breakdown functions, and leave the old ones alone. Their only callers are in `internal/web/pages.go`. Task 4 switches those callers over and deletes `CategoryCount`, the four old functions and their tests.

- [ ] **Step 1: Write the failing tests**

`TestDeviceCategoryCountsAt`: device `aa:bb:cc:dd:ee:ff`, `now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)`. Queries:
- current window: `tracking`×2, `telemetry`×1, `""`×1
- prior window (now−3d): `tracking`×1, `advertising`×2
- older than 8d (now−9d): `malware`×1 (excluded)
- another device in the current window: `tracking`×1 (excluded)

Want:
```go
[]CategoryWindowCount{
    {Category: "", Current: 1, Prior: 0},
    {Category: "advertising", Current: 0, Prior: 2},
    {Category: "telemetry", Current: 1, Prior: 0},
    {Category: "tracking", Current: 2, Prior: 1},
}
```

`TestNetworkCategoryCountsAt`: the same seed, with the other device's row counted (`tracking` Current 3).

`TestSourceCategoryCountsAt`: source `10.0.0.50` with `device_mac` `""` gets `tracking`×1 current. A device row with `source_ip` `10.0.0.50` is excluded. Want `[{tracking 1 0}]`.

`TestCategoryCountsAtEmpty`: an empty DB returns a zero-length slice and no error.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store -run 'CategoryCountsAt' -v`
Expected: FAIL, `undefined: CategoryWindowCount`

- [ ] **Step 3: Implement the three methods and `categoryCountsAt` in `internal/store/db.go`**

Use one `SELECT COALESCE(category,''), SUM(CASE current…), SUM(CASE prior…) … WHERE timestamp >= priorStart AND timestamp < now [AND actor.where] GROUP BY 1 ORDER BY 1`. `actorFilter`'s zero value matches every actor.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store -v`
Expected: PASS (the whole package, including the old breakdown tests, which still exist)

- [ ] **Step 5: Commit**

```bash
git add internal/store/db.go internal/store/db_test.go
git commit -m "Add per-category counts for current and prior windows"
```

---

### Task 2: Web — list matches, tracker badge and flags (pure view logic)

**Files:**
- Create: `internal/web/activity_view.go`
- Test: `internal/web/activity_view_test.go`

**Interfaces:**
- Consumes: `store.CategoryWindowCount` (Task 1), `store.Trend`, `store.Anomaly`, `anomalyBadge` and `anomalyExplanation` (existing in `pages.go`)
- Produces:
  ```go
  type listMatchRow struct {
      Category string  // "tracking"
      Tone     string  // "error" | "warn" | "success"
      Count    int
      Share    string  // "10.4%"
      Delta    string  // "▲ 2.1" | "▼ 0.4" | "─"
  }
  type listMatches struct {
      Rows      []listMatchRow
      Unmatched int // uncategorized + ""
  }
  func makeListMatches(counts []store.CategoryWindowCount) listMatches
  func categoryTone(category string) string
  func formatPoints(v float64) string            // round to 1 dp, drop ".0": 19 → "19", 2.06 → "2.1"
  func trackerBadge(t store.Trend) string         // "+19 vs prior 7d", "-3.5 vs prior 7d", "" when !t.HasPrior

  type actorFlag struct {
      Word        string // "tracker spike" | "volume spike" | "bypass likely" | "bypass suspected"
      Tone        string // "error" | "warn"
      Badge       string // anomalyBadge(a), e.g. "+12pp"
      Explanation string // anomalyExplanation(a)
      DeviceMAC   string
      DeviceName  string
  }
  func flagFor(a store.Anomaly) actorFlag
  func flagsByMAC(anomalies []store.Anomaly) map[string]actorFlag // highest priority wins: tracker spike > bypass > volume
  ```

Share is the category's current count over the sum of all current counts. Delta is the current share minus the prior share (prior share = the category's prior count over the sum of all prior counts), in points. `─` when the prior sum is 0, or when |delta| < 0.05.

- [ ] **Step 1: Write the failing tests**

`TestMakeListMatches`. Input:
```go
[]store.CategoryWindowCount{
    {"", 3208, 2000}, {"analytics", 267, 200}, {"tracking", 404, 250},
    {"uncategorized", 0, 50}, {"telemetry", 0, 0},
}
```
Want: two rows in the order tracking, analytics. Tracking: `Count 404`, `Share "10.4%"`, `Tone "error"`. `Unmatched 3208`. Assert tracking `Delta` starts with `"▲ "`. Prior sum 2500, so tracking prior share is 10.0% and the delta is +0.4, giving `"▲ 0.4"`.

`TestMakeListMatchesNoPrior`: all `Prior` zero → every `Delta == "─"`.

`TestMakeListMatchesOrder`: categories given alphabetically come back as `tracking, advertising, malware, analytics, telemetry`.

`TestTrackerBadge`:

| Input | Want |
|---|---|
| `{HasPrior:false}` | `""` |
| `{HasPrior:true, Change:19}` | `"+19 vs prior 7d"` |
| `{HasPrior:true, Change:-3.46}` | `"-3.5 vs prior 7d"` |

`TestFlagFor` (table): `tracker_spike` → `{"tracker spike","error"}`; `volume_spike` → `{"volume spike","warn"}`; `dns_bypass_likely` → `{"bypass likely","warn"}`; `dns_bypass_suspected` → `{"bypass suspected","warn"}`.

`TestFlagsByMACPriority`: the same MAC with `volume_spike` then `tracker_spike` → `"tracker spike"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web -run 'ListMatches|TrackerBadge|FlagFor|FlagsByMAC' -v`
Expected: FAIL, undefined identifiers

- [ ] **Step 3: Implement the signatures above in `internal/web/activity_view.go`**

`flagsByMAC` replaces `selectedDeviceAnomalies` and `anomalyClass`/`anomalyPriority` later (Task 4 deletes the old ones). Keep the old functions in place here so `pages.go` still builds.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/web -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/activity_view.go internal/web/activity_view_test.go
git commit -m "Add list-match, tracker badge and flag view helpers"
```

---

### Task 3: ElumKit foundation — copied source, theme, layout, Settings page

**Files:**
- Create: `internal/web/static/elumkit/` (copy of `packages/core-css/src/*`, plus `tabs.js`, `LICENSE`, and `fonts/OFL.txt` as copied)
- Create: `internal/web/static/css/app.css`
- Modify: `internal/web/static/embed.go` (`//go:embed css js templates elumkit`)
- Modify: `internal/web/static/templates/layout.html`
- Modify: `internal/web/static/templates/settings.html`
- Test: `internal/web/pages_test.go`

**Interfaces:**
- Produces:
  - Static URLs `/static/elumkit/index.css` and `/static/elumkit/tabs.js`, and `/static/css/app.css`
  - The `layout` template contract. Pages define `content` and optionally `page_scripts`. `page_styles` is removed. Layout `pageData.Active` values: `"activity"` | `"settings"`.
  - `app.css` classes used by later tasks:
    - `.ur-brand` (wordmark: `<b>Umber</b><span>relay</span>`)
    - `.ur-roster`, `.ur-pane` (two-column flex, per the spec's Layout section, with the pane first below 48rem via `order`)
    - `.ur-mini > i` (3px mini-meter; the width comes from an inline `style="width:N%"`)
    - `.ur-ref`, `.ur-ref .tick`, `.ur-ref .tick-label` (reference tick under an `elum-meter-track`, with the left offset as inline `style="left:N%"`)

How to get the files:
```bash
D=$(mktemp -d) && (cd "$D" && npm pack @baudsmithstudios/elumkit@0.4.0 --silent && tar xzf *.tgz)
mkdir -p internal/web/static/elumkit
cp -R "$D"/package/packages/core-css/src/* internal/web/static/elumkit/
cp "$D"/package/packages/core-js/src/tabs.js "$D"/package/LICENSE internal/web/static/elumkit/
```

- [ ] **Step 1: Write the failing tests**

`TestStaticServesElumKit`: `GET /static/elumkit/index.css` → 200 with body containing `@import`. `GET /static/elumkit/tabs.js` → 200.

`TestSettingsPageUsesElumKitLayout`: `GET /settings` body contains `/static/elumkit/index.css`, `/static/css/app.css`, `class="elum-navbar"`, `href="/settings" aria-current="page"`, `class="elum-switch"`, and does **not** contain `pico.min.css`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web -run 'ElumKit' -v`
Expected: FAIL (404 and missing strings)

- [ ] **Step 3: Copy ElumKit, write `app.css` (tokens per the spec's Palette table plus the classes above), rewrite `layout.html` and `settings.html`**

Layout: `elum-navbar` with the `.ur-brand` wordmark, `elum-navlinks` (Activity → `/`, Settings → `/settings`, `aria-current="page"` from `.Active`), and a status group with `window 24h`. Then `<main id="main">`, then the `elum-footer` (`Umberrelay {{version}} · forwarding DNS with attribution`; keep whatever version source the footer uses today). Load `/static/elumkit/index.css`, then `/static/css/app.css`, then `htmx.min.js`.

Settings, per the spec's Settings section. The enabled switch is a form posting to `/ui/lists/{id}/enabled` with hidden `enabled`. `hx-trigger="change"` on the switch's form keeps it one control. The refresh status is an `elum-alert` with `data-tone` from the existing status class: `status-error` → `error`, `status-ok` → `success`, `status-neutral` → no tone. Change `listRefreshStatusView` to return the tone directly, and update its callers.

- [ ] **Step 4: Run all web tests**

Run: `go test ./internal/web -v`
Expected: PASS, including the existing settings and list action tests. Home, Devices and Device Detail still render, unstyled. That is expected until Task 4.

- [ ] **Step 5: Commit**

```bash
git status
git add internal/web/static/elumkit internal/web/static/css/app.css internal/web/static/embed.go internal/web/static/templates/layout.html internal/web/static/templates/settings.html internal/web/pages.go internal/web/pages_test.go
git commit -m "Adopt ElumKit for layout and settings"
```

---

### Task 4: Activity page — roster, routes and pane header

**Files:**
- Create: `internal/web/static/templates/activity.html`
- Modify: `internal/web/pages.go` (replace `handleHome`, `handleDevices`, `handleDeviceDetail`, and their view structs)
- Modify: `internal/web/server.go` (routes, and `parsePages` page list → `{"activity", "settings"}`)
- Delete: `templates/home.html`, `templates/devices.html`, `templates/device_detail.html`
- Modify: `internal/store/db.go`, `internal/store/db_test.go` (delete the old `*CategoryBreakdown` functions, `CategoryCount` and their tests)
- Test: `internal/web/pages_test.go`

**Interfaces:**
- Consumes: Task 1's `*CategoryCountsAt`, Task 2's helpers, Task 3's layout and CSS classes
- Produces:
  ```go
  type rosterRow struct {
      ActorKey       string // "device:aa:…" | "source:10.0.0.50"; "" for network
      Href           string // "/" or "/devices/" + ActorKey
      Name           string
      Meta           string // "Roku · 412 q · tracker spike" | "Roku · 412 q · +19pp"
      TrackerPercent float64
      QueryCount     int
      Hot            bool   // has a tracker spike flag
      Selected       bool
  }
  type roster struct {
      Network rosterRow
      Devices []rosterRow // sorted tracker % desc, then name
      Sources []rosterRow
  }
  type trackerSummary struct {
      Percent  float64
      Badge    string  // trackerBadge(...)
      RefPct   float64
      RefLabel string  // "prior 7d 8.3%" | "network 10.4%"
      Matches  listMatches
  }
  type activityView struct {
      pageData
      Roster  roster
      Tab     string // "overview" | "domains" | "live"
      Network *networkPane   // nil unless network selected
      Detail  *privacyDetail // nil unless actor selected
  }
  type networkPane struct {
      Stats      store.DashboardStats
      Tracker    trackerSummary
      Flags      []actorFlag
      TopDomains []privacyDomainRow
  }
  func (s *Server) loadRoster(now time.Time, selectedKey string) (roster, map[string]actorFlag, error)
  func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) // serves "GET /{$}" and "GET /devices/{key}"
  ```
  `privacyDetail` gains `Tracker trackerSummary` and `Flag *actorFlag`, and loses `Breakdown`, `Title`, `ChartTitle`, `ChartDescription`, `QueryTrend` and `TrackerTrend` where nothing still reads them. `makePrivacyDetail`'s `categoryCounts` parameter becomes `[]store.CategoryWindowCount`.
- Template `activity.html` defines `content` with an `aside.ur-roster` and a `section.ur-pane`, following the spec's Roster section: filter field, sort segmented control, and groups. Each row has `.elum-row-value` in the error color when `.Hot`, and `.ur-mini` under the meta. The pane header follows the spec's Header section. This task renders the header, label form and system bar only. Tab panels come in Tasks 5 and 6.
- Network `RefLabel` uses `network trend .Previous`. Actor `RefLabel` uses `network trend .Current`. Get both from `s.db.LoadTrendsAt(now, "")`.

- [ ] **Step 1: Write the failing tests** (seed with `seedPrivacyPageData`)

- `TestActivityNetworkSelected`: `GET /` → 200. The body contains `Whole network` inside a link with `data-selected="true"`, `Living Room TV`, and the separator text `Devices`. It has no `Unattributed sources` heading, because the seed has no sources.
- `TestActivityDeviceSelected`: `GET /devices/device:aa:bb:cc:dd:ee:ff` → 200. The row with `href="/devices/device:aa:bb:cc:dd:ee:ff"` has `data-selected="true"`, `<h1>` contains `Living Room TV`, and the body contains the label form action `/ui/devices/aa:bb:cc:dd:ee:ff/label`.
- `TestActivitySourceIPv6Selected`: write a query with `SourceIP: "fe80::1"` and no MAC. `GET /devices/source:fe80::1` → 200. The `Unattributed sources` group is present and its row is selected.
- `TestDevicesListRouteIs404`: `GET /devices` → 404.
- Change `TestDeviceDetailUnknownDeviceRedirectsToDevicesList` to `…RedirectsToActivity`, with Location `/`.
- Change `TestDevicesListPageShowsUpdatedSourceLabel` to read `GET /`, and keep its assertion.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web -run 'Activity|DevicesListRoute|Redirects|SourceLabel' -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Register `GET /{$}` and `GET /devices/{key}` to `handleActivity`, and remove `GET /devices`. Route through `normalizeActorSelection`; invalid or unknown → `303` to `/`. Delete `homePageView`, `devicesListView`, `deviceDetailView`, `deviceTrendRow`/`makeDeviceTrendRows` (replaced by roster rows), `makeBreakdownRows`, `breakdownRowMeta`, `groupCategory`, `selectedDeviceAnomalies`, `anomalyClass`, `anomalyPriority`, `makeAnomalyRows`/`anomalyRow`, and the store's old breakdown functions, each after `grep` shows no remaining reference. Drop the `isHXRequest` fragment branch from the device route (nothing requests that fragment after this redesign). Delete `isHXRequest` too if it has no other caller.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git status
git add -u internal/ && git add internal/web/static/templates/activity.html
git commit -m "Merge home and device pages into roster activity view"
```

---

### Task 5: Overview tab — tabs, tracker meter, list matches, attention, top domains

**Files:**
- Modify: `internal/web/static/templates/activity.html`
- Modify: `internal/web/pages.go` (`parseTab`)
- Test: `internal/web/pages_test.go`

**Interfaces:**
- Consumes: `activityView` and `trackerSummary` (Task 4), `/static/elumkit/tabs.js` (Task 3)
- Produces:
  ```go
  func parseTab(raw string) string // "domains" | "live" | anything else → "overview"
  ```
  - Template blocks `tracker-meter` (takes `trackerSummary`) and `list-matches` (takes `listMatches`), both reused by the network and actor panes.
  - Tablist `elum-tabs` with buttons `id="tab-overview|tab-domains|tab-live"` controlling panels `id="panel-overview|panel-domains|panel-live"`. `aria-selected="true"` follows `.Tab`. Each panel starts with a heading for the no-JS fallback.
  - A chart container for Task 7: `<div class="tracker-chart" id="tracker-chart" data-chart-query="{{RangeQuery or ""}}"></div>` inside an `elum-card elum-card-labeled` titled `Tracker rate`, with an `elum-segmented` range control `name="chart-range"` (values `24h` (checked), `7d`, `30d`) in the card header and a subtitle element `data-chart-subtitle`.

Markup per the spec (Tracker rate, List matches and Tabs → Overview). The meter's `aria-valuetext` is `"{Percent}%, {RefLabel}"`. Percentages render with `printf "%.1f"`. Needs-attention alerts use `actorFlag` fields and link to `/devices/device:{DeviceMAC}`. Top domains: the first 6 of `TopDomains` as an `elum-list`.

- [ ] **Step 1: Write the failing tests**

- `TestOverviewShowsTrackerMeterAndMatches`: seeded `GET /` body contains `role="meter"` and `Matched a list`, contains `>tracking<` and `>analytics<` status labels and `>telemetry<` (the seed has telemetry), contains `matched no list`, and does **not** contain `unclassified</span>` inside the matches table. Assert by slicing the body between `Matched a list` and `matched no list`.
- `TestOverviewEmptyDatabase` (Review Focus 1): unseeded `GET /` → 200. The body contains `0.0%` and the empty-state text `No queries matched a list in the last 24 hours`, and does not contain `NaN` or `Inf`.
- `TestDeviceOverviewReferenceIsNetwork`: `GET /devices/device:aa:bb:cc:dd:ee:ff` contains `network ` followed by a percentage in the tick label.
- `TestParseTab` (Review Focus 4): `"domains"` → `"domains"`, `"live"` → `"live"`, `"bogus"` and `""` → `"overview"`.
- `TestTabQuerySelectsDomains`: `GET /?tab=domains` contains `id="tab-domains"` with `aria-selected="true"`.
- `TestNetworkOverviewNeedsAttentionEmpty`: seeded data without anomalies contains `No unusual activity`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web -run 'Overview|ParseTab|TabQuery|NeedsAttention' -v`
Expected: FAIL

- [ ] **Step 3: Implement the template blocks and `parseTab`, and load `tabs.js` with `defer` in `page_scripts`**

- [ ] **Step 4: Run all web tests**

Run: `go test ./internal/web -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/templates/activity.html internal/web/pages.go internal/web/pages_test.go
git commit -m "Add overview tab with tracker meter and list matches"
```

---

### Task 6: Domains and Live tabs

**Files:**
- Modify: `internal/web/static/templates/activity.html`, `internal/web/static/templates/components.html` (`label-edit`, `source-label-edit`, `domain-row` and `live-query-stream` rebuilt in ElumKit markup; ids and form actions unchanged)
- Modify: `internal/web/static/js/feed.js` (rows become `elum-feed` items)
- Modify: `internal/web/pages.go` (pagination links carry `tab=domains`; the "how identified" text)
- Test: `internal/web/pages_test.go`

**Interfaces:**
- Consumes: `privacyDetail`, `networkPane.TopDomains`, `parseTab`
- Produces:
  - `domain-row` keeps `id="domain-row-{{.Domain}}"`, the form `hx-post="/ui/overrides/{{.Domain}}" hx-target="closest tr" hx-swap="outerHTML"` and the hidden `actor_key`. The class text is `{{.ClassificationLabel}}` in an `elum-status-label` with `data-tone="{{categoryTone .Category}}"`. Register `categoryTone` as a template func in `parsePages`.
  - `func identificationNote(d store.Device) string`, e.g. `"MAC from the ARP table. Hostname roku-tv from DHCP. Vendor Roku from the MAC prefix."`, omitting the parts that are empty. Sources use the fixed text `"No MAC address was seen for this source, so it is tracked by IP."`.
  - Live panel: an `<p role="status">` stays in the page. `feed.js` puts `<span class="elum-spinner">streaming</span>` inside it while the EventSource is open and empties it on close. `<section id="live-query-stream" data-actor-key=…>` keeps the ids `live-query-domain-filter`, `live-query-category-filter`, `live-query-clear` and `live-query-feed` (now a `<ul class="elum-feed">`). `feed.js` renders `<li class="elum-feed-item" data-tone=…>` with an `elum-badge` (`[TRACKING]`), the domain, type, actor (network view only, i.e. when `data-actor-key` is empty) and `elum-feed-meta` time. The tone mapping in JS mirrors `categoryTone`.
  - The Domains tab `elum-query` filter is client-side over the current rows (domain substring and class equality). Put it in `feed.js`'s sibling script `activity.js`, created here (Task 8 extends it).

- [ ] **Step 1: Write the failing tests**

- `TestDomainsTabPaginationKeepsTab`: reuse the `TestDeviceDetailPageDomainPagination` seed. `GET /devices/device:aa…?tab=domains&page=2` contains `href="?tab=domains&amp;page=1"` (Prev) and `aria-selected="true"` on `tab-domains`.
- `TestDomainsTabPageBeyondRangeClamps` (Review Focus 5): `?tab=domains&page=999` → 200, and the status shows the last page.
- `TestUIOverrideReturnsUpdatedDomainRow` (existing): add assertions that the fragment contains `class="elum-status-label"` and `tracking · manual`.
- `TestIdentificationNote`: a device with hostname and vendor → the full sentence. A device with neither → only `MAC from the ARP table.`
- `TestLiveTabMarkup`: `GET /?tab=live` contains `id="live-query-feed"` with `class="elum-feed"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web -run 'DomainsTab|UIOverride|IdentificationNote|LiveTab' -v`
Expected: FAIL

- [ ] **Step 3: Implement the templates, `identificationNote`, the pagination links and the `feed.js` row rendering**

- [ ] **Step 4: Run all web tests**

Run: `go test ./internal/web -v`
Expected: PASS

- [ ] **Step 5: Manual check of the live feed**

Run `scripts/umberrelay-demo.sh`, open `/?tab=live` and a device's `?tab=live`. Confirm rows render as feed items with category badges and the filters still narrow them. Demo mode has no live writer, so confirm only that the "Waiting for live queries…" empty row renders and the filter controls are present. Note this in the commit body.

- [ ] **Step 6: Commit**

```bash
git status
git add internal/web/static/templates internal/web/static/js/feed.js internal/web/static/js/activity.js internal/web/pages.go internal/web/pages_test.go
git commit -m "Add domains and live tabs in ElumKit markup"
```

---

### Task 7: Tracker rate chart — one series, one axis

**Files:**
- Modify: `internal/web/static/js/charts.js` (rewrite)

**Interfaces:**
- Consumes: `#tracker-chart[data-chart-query]`, `input[name=chart-range]` and `[data-chart-subtitle]` (Task 5), `/api/activity?range=24h|7d|30d[&actor=…]` → `[{timestamp, total, tracker}]`
- Produces: nothing other tasks consume

Behavior is fixed by the spec's Chart section:
- per bucket, `rate = tracker / total × 100`; `null` when `total == 0`, and the line breaks there
- the y-axis is 0 to a nice max ≥ the highest rate, in `%`
- a dashed average line at `Σtracker / Σtotal × 100`, labeled `avg N%`
- the peak point gets a marker and a label
- the subtitle text is `by hour` for `24h`, otherwise `by day`
- a crosshair and tooltip on pointer move and on keyboard focus: the canvas gets `tabindex="0"`, Left/Right move between buckets, and the tooltip shows the time, the rate and `N queries`
- colors from `getComputedStyle` on `--elum-color-error`, `--elum-color-muted` and `--elum-color-border`
- redraw on range change and on resize (keep the existing `ResizeObserver` approach)

No JS test harness exists (spec, Testing).

- [ ] **Step 1: Rewrite `charts.js` to the behavior above. Remove `axis: 'right'` and all dual-axis code.**

- [ ] **Step 2: Manual verification**

Run `scripts/umberrelay-demo.sh`. On `/`, check:
- one y-axis labeled in %
- the average line and peak label are visible and don't collide
- switching to 7d changes the subtitle to `by day`
- hover and arrow keys move the tooltip
- the chart redraws on a window resize

Repeat on one device page.

- [ ] **Step 3: Run the Go tests (the templates reference this script)**

Run: `go test ./internal/web`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/web/static/js/charts.js
git commit -m "Draw tracker rate as a single-axis chart"
```

---

### Task 8: Roster behavior — filter, sort, live refresh

**Files:**
- Modify: `internal/web/static/js/activity.js`
- Delete: `internal/web/static/js/devices.js`
- Modify: `internal/web/static/templates/activity.html` (`data-*` hooks on rows)

**Interfaces:**
- Consumes: roster markup from Task 4. Each actor row is `<a class="elum-row" data-actor-key data-name data-tracker-percent data-query-count>`, with `.elum-row-value`, `.elum-row-meta` and `.ur-mini > i`. Add these `data-*` attributes in this task. Also consumes `/api/actors` (`key`, `name`, `query_count`, `tracker_percent`) and `/api/queries/stream?limit=1`.
- Produces: nothing other tasks consume

Behavior:
- **Filter field** (`#roster-filter`): hides rows whose `data-name`, vendor/IP meta or key don't contain the text, case-insensitive.
- **Sort** (`input[name=roster-sort]`, values `tracker|volume|name`): reorders rows within each group (Devices, Sources). The Whole network row never moves.
- **Live refresh:** port `devices.js`'s SSE poke → `/api/actors` throttle loop. Update each row's value text, the mini-meter width and the `data-*` values, then re-apply sort and filter. New actors are not inserted; they appear on the next page load. *(My call: this keeps the server as the only row renderer.)*

- [ ] **Step 1: Add the `data-*` attributes in `activity.html`, and extend the existing `TestActivityDeviceSelected` to assert `data-tracker-percent=` and `data-actor-key="device:aa:bb:cc:dd:ee:ff"` are present**

- [ ] **Step 2: Run that test to verify it fails, then add the attributes, then verify it passes**

Run: `go test ./internal/web -run TestActivityDeviceSelected -v`

- [ ] **Step 3: Implement the roster behavior in `activity.js` and delete `devices.js`**

- [ ] **Step 4: Manual verification**

In demo mode, on `/`:
- typing `roku` leaves only the Roku row
- sorting by name orders the rows alphabetically
- the Whole network row stays first

- [ ] **Step 5: Commit**

```bash
git status
git add internal/web/static/js/activity.js internal/web/static/templates/activity.html internal/web/pages_test.go
git rm internal/web/static/js/devices.js
git commit -m "Add roster filter, sort and live refresh"
```

---

### Task 9: Remove Pico and old styles, update docs

**Files:**
- Delete: `internal/web/static/css/{pico.min.css,theme.css,components.css,home.css,devices.css,device_detail.css,settings.css,uplot.min.css}` and `internal/web/static/js/uplot.min.js`
- Modify: `README.md`, `docs/TROUBLESHOOTING.md`, `NOTICE`, `CHANGELOG.md`
- Test: `internal/web/pages_test.go`

- [ ] **Step 1: Write the failing test**

`TestNoPicoAssetsServed`: `GET /static/css/pico.min.css` → 404 and `GET /static/js/uplot.min.js` → 404.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/web -run TestNoPicoAssetsServed -v`
Expected: FAIL (200)

- [ ] **Step 3: `grep -rn` each file name across `internal/`; delete each one with no references**

- [ ] **Step 4: Update the docs**

- `README.md` line 27: the *Web UI* bullet now names the Activity view (an actor roster with a network or per-actor detail pane: tracker rate against a baseline, list matches, tracker-rate chart, domains with overrides, live stream) and the Settings page.
- `README.md` line 309 and `docs/TROUBLESHOOTING.md` line 160: "Devices page" → "the Activity roster's *Unattributed sources* group". Line 347: "Home, Devices, and Settings pages" → "Activity and Settings pages".
- `docs/TROUBLESHOOTING.md` line 116: "the Home page loads" → "the Activity page loads".
- `NOTICE`: add "This product includes ElumKit (https://github.com/baudsmithstudios/elumkit), MIT License, and JetBrains Mono, SIL Open Font License 1.1."
- `CHANGELOG.md` under `## [Unreleased]`: a `### Changed` entry for the roster UI on ElumKit, the meter and list matches, the single-axis chart, and `/devices` removed. A `### Removed` entry for Pico and uPlot.
- Leave `assets/readme-home.png` alone (retake after the tweak round).

- [ ] **Step 5: Run everything**

Run: `go test ./... && go vet ./...`
Expected: PASS with no vet output

- [ ] **Step 6: Commit**

```bash
git status
git add -u && git add internal/web/pages_test.go
git commit -m "Remove Pico and old page styles; update docs for roster UI"
```
