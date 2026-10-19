(function () {
    // Narrows the domain rows already on this page; pagination still pages the full set.
    function bindDomainFilter() {
        const form = document.getElementById('domain-filters');
        const rows = document.getElementById('domain-rows');
        const text = document.getElementById('domain-filter');
        const cls = document.getElementById('domain-class-filter');

        function apply() {
            const needle = text.value.trim().toLowerCase();
            const wanted = cls.value;
            rows.querySelectorAll('tr[id^="domain-row-"]').forEach(function (row) {
                const domain = row.id.slice('domain-row-'.length).toLowerCase();
                const category = row.querySelector('select[name="category"]').value;
                row.hidden = (needle && domain.indexOf(needle) === -1) || (wanted && category !== wanted);
            });
        }

        form.addEventListener('submit', function (event) {
            event.preventDefault();
        });
        text.addEventListener('input', apply);
        cls.addEventListener('change', apply);
        document.body.addEventListener('htmx:afterSwap', apply);
    }

    function rosterRows() {
        return Array.prototype.slice.call(document.querySelectorAll('.ur-roster .elum-row[data-actor-key]'));
    }

    function applyRoster() {
        const needle = document.getElementById('roster-filter').value.trim().toLowerCase();
        const sort = document.querySelector('input[name="roster-sort"]:checked').value;
        rosterRows().forEach(function (row) {
            const haystack = [row.dataset.name, row.dataset.actorKey, row.querySelector('.elum-row-meta').textContent].join(' ').toLowerCase();
            row.parentElement.hidden = needle !== '' && haystack.indexOf(needle) === -1;
        });
        document.querySelectorAll('.ur-roster [data-roster-group]').forEach(function (group) {
            Array.prototype.slice.call(group.children).sort(function (leftItem, rightItem) {
                const left = leftItem.firstElementChild.dataset;
                const right = rightItem.firstElementChild.dataset;
                if (sort === 'name') {
                    return left.name.localeCompare(right.name);
                }
                const key = sort === 'volume' ? 'queryCount' : 'trackerPercent';
                return Number(right[key]) - Number(left[key]) || left.name.localeCompare(right.name);
            }).forEach(function (item) {
                group.appendChild(item);
            });
        });
    }

    // New actors appear on the next page load; live updates only touch rows already rendered.
    function refreshRoster(state) {
        if (state.inFlight) {
            state.pending = true;
            return;
        }
        state.inFlight = true;
        fetch('/api/actors')
            .then(function (response) {
                if (!response.ok) {
                    throw new Error('request failed');
                }
                return response.json();
            })
            .then(function (actors) {
                const byKey = {};
                actors.forEach(function (actor) {
                    byKey[actor.key] = actor;
                });
                rosterRows().forEach(function (row) {
                    const actor = byKey[row.dataset.actorKey];
                    if (!actor) {
                        return;
                    }
                    const percent = Math.min(100, Math.max(0, Number(actor.tracker_percent) || 0));
                    row.dataset.trackerPercent = percent.toFixed(2);
                    row.dataset.queryCount = String(actor.query_count);
                    row.querySelector('.elum-row-value').textContent = Math.round(percent) + '%';
                    row.querySelector('.ur-mini > i').style.width = percent.toFixed(1) + '%';
                });
                applyRoster();
            })
            .catch(function () {})
            .finally(function () {
                state.inFlight = false;
                if (state.pending) {
                    state.pending = false;
                    refreshRoster(state);
                }
            });
    }

    function bindRoster() {
        const filter = document.getElementById('roster-filter');
        filter.form.addEventListener('submit', function (event) {
            event.preventDefault();
        });
        filter.addEventListener('input', applyRoster);
        document.querySelectorAll('input[name="roster-sort"]').forEach(function (input) {
            input.addEventListener('change', applyRoster);
        });

        if (typeof window.EventSource === 'undefined') {
            return;
        }
        const state = { inFlight: false, pending: false, timer: null };
        const source = new EventSource('/api/queries/stream?limit=1');
        source.addEventListener('query', function () {
            clearTimeout(state.timer);
            state.timer = setTimeout(function () {
                refreshRoster(state);
            }, 250);
        });
        window.addEventListener('beforeunload', function () {
            source.close();
        }, { once: true });
    }

    document.addEventListener('DOMContentLoaded', bindDomainFilter);
    document.addEventListener('DOMContentLoaded', bindRoster);
})();
