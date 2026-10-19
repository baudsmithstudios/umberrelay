# Roster UI on ElumKit

Status: draft for review · 2026-10-19

## Intent

Replace the web UI's look and page structure. Pico CSS goes away and
[ElumKit](https://www.npmjs.com/package/@baudsmithstudios/elumkit) becomes the
component library. The Home, Devices and Device Detail pages merge into a
single **roster** layout: a persistent list of every network actor on the left,
and a detail pane for whichever one is selected on the right. The whole
network is the first entry in the list and is selected by default.

The question Umberrelay answers best is "which device is talking to trackers,
and how much?" (README, *Where It Fits Best*). Today that answer is spread over
three pages, and the device comparison lives on a separate page from the
detail. In the roster layout, comparing devices takes no clicks, and the
detail pane explains one actor at a time.

The second goal is clearer encoding. The three-tone breakdown bar is removed.
Unclassified traffic, which is most queries, crowded out the categories that
matter. Its amber matched the link color. It also gave no sense of whether a
rate is high or low. It is replaced by a tracker-rate meter with a reference
tick and a short table of list matches.

The visual reference is design **B · Roster** on the canvas
https://claude.ai/artifact/7AQGfp9tume4Y7FkAQsD5v (artboards
`Roster.dc.html` and `RosterDevice.dc.html`). The canvas shows sample data and
static controls. This document wins wherever the two disagree.

## Goals

- ElumKit is the only UI library. `elum-*` classes are used for every
  component it covers.
- Keep the current umber palette, mapped onto ElumKit tokens.
- One roster layout serves both the network view and every actor's view.
- The tracker rate is shown as a value set against context (a previous
  baseline, and the network average on actor views). It is never a slice of a
  stacked bar.
- Category counts list only categories that matched a blocklist or an
  override. Everything else is one muted line.
- The trend chart has a single y-axis.
- Every current capability stays reachable: labels, domain overrides, the live
  stream, anomalies and bypass flags, list management and retention settings.

## Non-goals

- No JSON API (`/api/*`) changes are planned. The design doesn't need any,
  but changes are allowed if the build turns one up.
- No change to DNS, the pipeline, classification or retention behavior.
- No light theme or theme switcher.
- No filter-prompt syntax (the `device:` / `category:` box from design C).
- No new client framework or build step. Pages stay server-rendered Go
  templates with HTMX and small vanilla scripts.

## Anti-goals (a failure even if it works)

- A colored bar or legend where "unclassified" takes up space alongside real
  categories.
- Any chart with two y-scales.
- State that relies on color alone. Category, tone and trend always carry a
  text label, badge text or arrow as well.
- Restyling ElumKit components with local CSS overrides until they no longer
  look like ElumKit. Theme by tokens. App CSS covers only what ElumKit lacks.

## Constraints

- ElumKit is distributed as source you copy and own (its README, *Install
  From npm*). Copy `packages/core-css/src/` from `@baudsmithstudios/elumkit`
  **0.4.0** into the embedded static tree, keeping its folder structure, with
  its `LICENSE` and the font's `OFL.txt`. `packages/core-js/src/tabs.js` is
  copied too. Do not use `dist/elumkit.css`; the copied `index.css` imports
  the parts.
- The static tree is embedded with `go:embed` (`internal/web/static/embed.go`).
  Every new file must be inside it.
- ElumKit needs `:has()`, the Popover API and CSS anchor positioning (for
  action menus), and invoker commands (for dialogs). This design uses no
  dialogs and no action menus, so the floor is modern evergreen browsers with
  `:has()`.
- `mutationOriginGuard` and the `/ui/*` HTMX form endpoints stay as they are.
  Forms keep posting to the same routes with the same field names.
- The project rules apply: TDD for Go changes, smallest reasonable change, no
  backward-compatibility shims without Thom's approval.

## Palette (decided)

Override ElumKit's color tokens on `:root` in the app stylesheet. Values come
from the current `theme.css`:

| Token | Value | Was |
|---|---|---|
| `--elum-color-bg` | `#0e0b04` | `--pico-background-color` |
| `--elum-color-surface` | `#151006` | `--pico-card-background-color` |
| `--elum-color-fg` | `#f0e6c8` | `--pico-color` |
| `--elum-color-muted` | `#b09470` | `--pico-muted-color` |
| `--elum-color-border` | `#5a4528` | `--umberrelay-panel-border` |
| `--elum-color-accent` | `#f5a623` | `--umberrelay-accent` |
| `--elum-color-success` | `#1dbd5c` | `--umberrelay-positive` |
| `--elum-color-warn` | `#e08700` | `--umberrelay-accent-deep` |
| `--elum-color-error` | `#ff4f4f` | `--umberrelay-critical` |
| `--elum-color-info` | `#c8b88a` | `--umberrelay-title-relay` |
| `--elum-color-focus` | `#ffbe4d` | `--pico-primary-hover` |

The wordmark keeps its two tones: "Umber" in accent, "relay" in
`#c8b88a`.

Category tones map onto ElumKit's tone attribute:

| Category | Tone |
|---|---|
| tracking, malware | `error` |
| advertising | `warn` |
| analytics, telemetry | `success` |

Tones are only ever applied to `elum-status-label` or `elum-badge`, which
carry the category name as text.

## Information architecture (decided)

Top navbar (`elum-navbar`): wordmark, then links **Activity** (to `/`) and
**Settings**. The status group on the right shows only the reporting window,
`window 24h`. *(My call: no `dns [UP]` item. The web server has no view of the
listener's state, so the item would claim a status nobody checks.)*

| Route | Renders |
|---|---|
| `GET /` | Roster layout, **Whole network** selected |
| `GET /devices/{actorKey}` | Roster layout, that actor selected |
| `GET /settings` | Settings page (retention and lists, as today) |

`/devices` (the card grid) is removed; the roster replaces it. `GET /devices`
is not redirected and returns 404. Umberrelay is still in development, so URL
and API changes need no compatibility shims.

Selecting a roster entry is a plain link and a full page load. No HTMX
pane-swapping in this pass.

An unknown or malformed actor key on `/devices/{actorKey}` redirects to `/`
(today it redirects to `/devices`).

## Flags (decided)

Flags come only from today's sources: `DeviceAnomalies` and
`DeviceBypassSignalsAt`, merged and prioritized as `loadAttentionAnomalies`
and `anomalyPriority` do now. There is no "new device" flag. Each anomaly type
has a fixed word and tone, so no two flags differ by color alone:

| Anomaly type | Flag word | Tone | Badge text (existing `anomalyBadge`) |
|---|---|---|---|
| `tracker_spike` | `tracker spike` | `error` | `[+Npp]` |
| `volume_spike` | `volume spike` | `warn` | `[N.Nx]` or `[+N]` |
| `dns_bypass_likely` | `bypass likely` | `warn` | `[LIKELY]` |
| `dns_bypass_suspected` | `bypass suspected` | `warn` | `[SUSPECTED]` |

The explanation sentences stay as `anomalyExplanation` writes them.

## The roster (left column)

From top to bottom:

1. **Filter field.** `elum-prompt-field` with a `▶` prompt. It matches name,
   vendor or IP as you type, client-side, as `devices.js` does today.
2. **Sort.** `elum-segmented` with tracker %, volume and name. Client-side;
   tracker % is the default.
3. **Whole network** row.
4. `elum-separator-labeled` **Devices**, then one row per device.
5. `elum-separator-labeled` **Unattributed sources**, then one row per
   source-IP actor. The group is omitted when there are none.

Each row is an `elum-list` / `elum-row` link:

- title: display name (same rules as today's `deviceTrendDisplayName`)
- meta: vendor (or source IP) · query count · tracker trend arrow, or the
  actor's flag word (see Flags) when it has one
- a 3px mini-meter under the meta, filled to the actor's tracker percent on a
  fixed 0–100% scale, one neutral color (`--elum-color-muted`) for every row
- value: tracker percent. It is shown in the error color when the actor has
  a tracker-spike anomaly. Otherwise it is shown in the foreground color.
- the selected row has `data-selected="true"` and `aria-current="true"`

The mini-meter is app CSS (ElumKit has no inline micro-meter). It is
`aria-hidden`, because the percentage next to it is the accessible value.

The live roster refresh that `devices.js` does today (SSE poke, then
`/api/actors`) carries over to rows: percentages, counts and order update in
place.

## The detail pane (right column)

### Header

- eyebrow: `Whole network · last 24h · N queries · M domains`, or for an
  actor `Device` / `Source` plus its flag word as an `elum-status-label` in the
  flag's tone
- `h1`: display name, or "N% of DNS queries hit known trackers across M
  devices" for the network
- actor views only: the label form (`elum-field-float` with a Save button),
  posting to the existing `/ui/devices/{mac}/label` or
  `/ui/sources/{ip}/label`
- actor views only: an `elum-system-bar` of facts (hostname as the brand,
  then vendor, MAC, IP, last seen, and `24h` query and domain counts)

### Tracker rate

An `elum-meter` filled to the current 24h tracker percent with
`data-tone="error"`. The header's value text reads `31%` followed by an
`elum-badge` such as `[+19 vs prior 7d]`.

Under the track, a 2px reference tick with a text label:

- network view: the prior-7-day tracker percent (`prior 7d 8.3%`)
- actor view: the network's current tracker percent (`network 10.4%`)

The track carries `aria-valuetext` with both numbers ("31%, network 10.4%").
The tick is `aria-hidden`.

"vs prior 7d" uses the existing `Trend` from `LoadTrendsAt`: current 24h
against the 7 days before that. The badge text replaces today's
"vs previous period".

### List matches

An `elum-table` titled **Matched a list**, with columns category, queries,
share and vs prior 7d:

- one row per category in `tracking, advertising, malware, analytics,
  telemetry` order, with rows of zero count omitted
- the category cell is an `elum-status-label` with that category's tone
- share is the category's percent of the actor's (or network's) 24h queries
- vs prior 7d is the change in that share, in percentage points, against the
  prior 7 days, shown as `▲ 2.1`, `▼ 0.4` or `─`

Below the table, one muted line: `N queries matched no list`. This counts
`uncategorized` plus empty category.

When nothing matched, the table is replaced by an `elum-empty` saying no
queries matched a list in the last 24 hours.

This needs per-category counts for both windows. Today
`NetworkCategoryBreakdown` collapses everything into three buckets, and
`categoryBreakdown` has no prior window. Replace both with one store query
that returns, per raw category, the count in the current 24h and the prior
7 days, for the network or one actor. Its exact shape is the builder's call.
Telemetry currently falls into "unclassified" (`groupCategory`); in this
design it gets its own row.

### Tabs

`elum-tabs` with **Overview**, **Domains** and **Live**, enhanced by the
copied `tabs.js`. The selected tab comes from `?tab=overview|domains|live`
(default `overview`) and is rendered server-side as `aria-selected="true"`, so
reloads and pagination links keep their place. Pagination links on the Domains
tab carry `tab=domains`. Without JavaScript, all three panels render stacked,
each with its own heading (ElumKit's documented fallback).

**Overview**

- the tracker rate meter and the list-matches table side by side
  (`elum-grid`), stacking on narrow screens
- **Tracker rate** chart (below)
- network view only: **Needs attention** as a stack of `elum-alert`s, one
  per flag, in today's priority order. Each alert has the flag's tone, the flag
  word and its badge (see Flags), the explanation sentence, and a link to the
  actor. When there are none, show `elum-empty` ("No unusual activity").
- network view only: **Top domains** as an `elum-list` (domain, a status
  label for its class, reach, and count), top 6

**Domains**

- network view: the top-domains table (domain, class, queries, reach, and an
  override action)
- actor view: that actor's paginated domain table (domain, queries, and an
  override action)
- the override action is the existing form (category `elum-select` plus a
  tertiary **Set** button, posting to `/ui/overrides/{domain}` and swapping
  the row). The class text stays exactly as `classificationLabel` produces
  it today (`tracking · EasyPrivacy`, `tracking · manual`), shown inside an
  `elum-status-label` with the category's tone.
- an `elum-query` filter row above the table for domain text and class.
  Client-side, filtering the current page only.
- `elum-pagination` as today's pagination
- actor view: an `elum-disclosure` **How this device was identified**, built
  from what is known (ARP MAC, DHCP hostname, OUI vendor). For a source actor,
  it explains that no MAC was available.

**Live**

- the live query stream for this actor, or for everything on the network view
- `elum-query` filters: domain text, category select, and a Clear button
  (same behavior as today's `feed.js`)
- rows as `elum-feed` items: tone by category, an `elum-badge` with the
  category, domain, record type, the actor name (network view only) and the
  time in `elum-feed-meta`
- an `elum-spinner` with "streaming" inside a `role="status"` element while
  connected

### Chart: tracker rate

A single series: tracker percent per bucket (`tracker / total × 100`) from
`/api/activity`. That endpoint returns hourly buckets for 24h and daily
buckets for 7d and 30d, so the card subtitle and tooltip follow the bucket
size ("by hour" / "by day"). The API is unchanged. One y-axis in percent, with a nice max rounded up from
the data. A dashed horizontal line shows the range's overall rate
(`sum tracker / sum total`), labeled `avg N%`. The peak point gets a marker
and a direct label. Buckets with zero queries are gaps, not zeros.

The range control is an `elum-segmented` with 24h, 7d and 30d (24h default).
It sits in the card header and replaces the current button row.

On hover or focus, a crosshair and tooltip show the bucket time, the rate and
the bucket's query count.

Colors come from the CSS tokens (`--elum-color-error` for the line,
`--elum-color-muted` for axes and the average line, `--elum-color-border` for
grid lines). Canvas drawing stays in `charts.js`. It is rewritten for one
series, and the two-axis code goes away.

## Settings page

Same content and endpoints as today, rebuilt from ElumKit parts:

- **Retention**: two `elum-field` number inputs and a primary Save button
- **Classification lists**: refresh button, refresh status as an `elum-alert`
  (tone from the current status class), and the lists as an `elum-table`
  with enabled shown as an `elum-switch` that posts on change, plus a Remove
  button
- **Add list**: URL, name and category fields, and an Add button

## Layout and responsiveness

- The page is a full-width two-column flex row. The roster is `flex: 1 1
  340px; max-width: 420px` with a right border, and the pane is
  `flex: 999 1 560px; min-width: 0`.
- Below 48rem the columns stack, with the **pane first** and the roster
  after it, so a selected actor's detail is what you see.
- Wide tables sit in `elum-table-wrap`, which ElumKit linearizes on narrow
  screens using `data-label`.
- Footer (`elum-footer`): `Umberrelay {version} · forwarding DNS with
  attribution`.

## What gets removed

Pico (`pico.min.css`), `theme.css`, `components.css`, `home.css`,
`devices.css`, `device_detail.css` and `settings.css`. Also the unused uPlot
files (`uplot.min.js`, `uplot.min.css`, which nothing references today), the
`home.html`, `devices.html` and `device_detail.html` templates, and the view
code that only they use (for example `makeBreakdownRows`, `breakdownRowMeta`
and `groupCategory`). The builder confirms each is unreferenced before
deleting it.

## Docs to update

- `README.md`: the *Web UI* feature bullet, the architecture tree line
  ("Home, Devices, and Settings pages"), the troubleshooting line that points
  at the Devices page, and the screenshot `assets/readme-home.png` (retake
  with demo data after the tweak round, not in this pass)
- `docs/TROUBLESHOOTING.md`: references to the Home and Devices pages
- `NOTICE`: add ElumKit (MIT) and JetBrains Mono (SIL OFL 1.1) attributions
- `CHANGELOG.md`: an entry under the next version

## Testing

Go handler and store tests, following the existing `pages_test.go` and
store test patterns, against a real SQLite database:

- the store's per-category current and prior counts, for the network and for
  one actor (both windows, and empty windows)
- `/` renders the roster with Whole network selected, and the matched-list
  rows exclude uncategorized
- `/devices/{actorKey}` renders the roster with that row selected, and the
  pane for that actor (device and source cases)
- `?tab=domains` selects the Domains tab, and pagination links keep it
- the share delta formats as `▲`, `▼` or `─`
- settings forms still post to the same routes (existing tests keep passing)

JavaScript (roster filter and sort, the chart, the live feed) has no test
harness in the repo. Verify it by running the binary with `-demo-data`
(`scripts/umberrelay-demo.sh`) and checking each page in a browser.

## Left to the builder

- The store query's signature, and how the share delta is computed and
  rounded
- How the vendored ElumKit files are laid out under `static/`, and the app
  stylesheet's name
- Template file names and how partials are split
- Chart drawing details (padding, tick count, label collisions) within the
  rules above
- Whether the roster's live refresh reuses `devices.js` or is folded into a
  new script

## Calls made while writing (check these)

- No `dns [UP]` status in the navbar (see Information architecture).
- Flags are today's four anomaly types with fixed words and tones. There is no
  "new device" flag.
- The chart subtitle follows the API's bucket size. No hourly 7d/30d data.
- Domain class text keeps today's `classificationLabel` format.
- Unknown actors redirect to `/`.
- Lists stay on the Settings page. There is no separate Lists nav item, unlike
  the mockup.
- The network view has a Domains tab (the top-domains table with overrides),
  so it has the same three tabs as an actor view.
- Below 48rem the detail pane comes before the roster.
- Hover tooltips on the chart are added. Today's chart has none.
