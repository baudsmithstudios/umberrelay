(function () {
    var liveQueryStream = {
        source: null,
        signature: '',
    };

    function chartColor(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }

    function renderChart(container, range) {
        var query = container.dataset.chartQuery || '';
        var url = '/api/activity?range=' + encodeURIComponent(range);
        if (query) {
            url += '&' + query.replace(/^\?/, '').replace(/^&/, '');
        }

        fetch(url)
            .then(function (response) {
                if (!response.ok) {
                    throw new Error('request failed');
                }
                return response.json();
            })
            .then(function (buckets) {
                var times = buckets.map(function (bucket) { return bucket.timestamp; });
                var totals = buckets.map(function (bucket) { return bucket.total; });
                var trackerRate = buckets.map(function (bucket) {
                    if (!bucket.total) {
                        return 0;
                    }
                    return bucket.tracker / bucket.total * 100;
                });

                container.innerHTML = '';
                if (container._uplot) {
                    container._uplot.destroy();
                }

                var axisStroke = chartColor('--pico-card-border-color') || '#a27f46';
                var axisOpts = {
                    stroke: axisStroke,
                    grid: { show: false },
                    ticks: { show: true, stroke: axisStroke, width: 1.25 / devicePixelRatio, size: 6 },
                    font: '11px "Courier Prime", "Courier New", monospace',
                    labelFont: '11px "Courier Prime", "Courier New", monospace',
                    labelSize: 16,
                };

                container._uplot = new uPlot({
                    width: container.clientWidth || 720,
                    height: 280,
                    scales: {
                        x: { time: true },
                        rate: { auto: true },
                        volume: { auto: true },
                    },
                    legend: { show: true, live: false },
                    cursor: { show: false },
                    axes: [
                        Object.assign({}, axisOpts),
                        Object.assign({ scale: 'rate', label: 'Tracker %' }, axisOpts),
                        Object.assign({ scale: 'volume', label: 'Queries', side: 1 }, axisOpts),
                    ],
                    series: [
                        {},
                        { label: 'Tracker rate', scale: 'rate', stroke: chartColor('--umberrelay-chart-tracker'), width: 2, points: { show: false } },
                        { label: 'Queries', scale: 'volume', stroke: chartColor('--umberrelay-chart-total'), width: 2, points: { show: false } },
                    ],
                }, [times, trackerRate, totals], container);
            })
            .catch(function () {
                container.textContent = 'Failed to load chart data';
            });
    }

    function bindChart(container) {
        if (!container || container.dataset.bound === 'true') {
            return;
        }
        container.dataset.bound = 'true';

        var controls = document.querySelector('.range-controls[data-chart-id="' + container.id + '"]');
        var defaultRange = '7d';
        if (controls) {
            controls.querySelectorAll('button[data-range]').forEach(function (button) {
                button.addEventListener('click', function () {
                    controls.querySelectorAll('button').forEach(function (candidate) {
                        candidate.classList.add('outline');
                    });
                    button.classList.remove('outline');
                    renderChart(container, button.dataset.range);
                });
                if (!button.classList.contains('outline')) {
                    defaultRange = button.dataset.range;
                }
            });
        }

        renderChart(container, defaultRange);
    }

    function bindDeviceList() {
        var search = document.getElementById('device-search');
        var sort = document.getElementById('device-sort');
        var list = document.getElementById('device-list');
        if (!search || !sort || !list || list.dataset.bound === 'true') {
            return;
        }
        list.dataset.bound = 'true';

        function applyListState() {
            var query = search.value.toLowerCase();
            var rows = Array.prototype.slice.call(list.querySelectorAll('.device-item:not(.all-devices)'));
            rows.forEach(function (row) {
                var name = (row.dataset.deviceName || '').toLowerCase();
                row.hidden = query !== '' && name.indexOf(query) === -1;
            });

            rows.sort(function (left, right) {
                if (sort.value === 'name') {
                    return (left.dataset.deviceName || '').localeCompare(right.dataset.deviceName || '');
                }
                if (sort.value === 'volume') {
                    return Number(right.dataset.queryCount || '0') - Number(left.dataset.queryCount || '0');
                }
                return Number(right.dataset.trackerPercent || '0') - Number(left.dataset.trackerPercent || '0');
            }).forEach(function (row) {
                list.appendChild(row);
            });
        }

        search.addEventListener('input', applyListState);
        sort.addEventListener('change', applyListState);
        applyListState();
    }

    function closeLiveQueryStream() {
        if (liveQueryStream.source) {
            liveQueryStream.source.close();
            liveQueryStream.source = null;
        }
        liveQueryStream.signature = '';
    }

    function escapeHTML(value) {
        return String(value || '')
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;');
    }

    function formatLiveActor(query) {
        if (query.device_mac) {
            return query.device_mac;
        }
        if (query.source_ip) {
            return 'Unattributed · ' + query.source_ip;
        }
        return 'Unknown';
    }

    function formatLiveTime(unixSeconds) {
        if (!unixSeconds) {
            return 'now';
        }
        var ts = Number(unixSeconds);
        if (!Number.isFinite(ts)) {
            return 'now';
        }
        var date = new Date(ts * 1000);
        if (Number.isNaN(date.getTime())) {
            return 'now';
        }
        return date.toLocaleTimeString();
    }

    function appendLiveQueryRow(feed, query) {
        if (feed.dataset.hasRows !== 'true') {
            feed.innerHTML = '';
            feed.dataset.hasRows = 'true';
        }

        var row = document.createElement('tr');
        row.innerHTML =
            '<td data-label="Time">' + escapeHTML(formatLiveTime(query.timestamp)) + '</td>' +
            '<td data-label="Actor">' + escapeHTML(formatLiveActor(query)) + '</td>' +
            '<td data-label="Domain">' + escapeHTML(query.domain) + '</td>' +
            '<td data-label="Type">' + escapeHTML(query.query_type || '') + '</td>' +
            '<td data-label="Category">' + escapeHTML(query.category || 'unclassified') + '</td>';
        feed.insertBefore(row, feed.firstChild);

        while (feed.children.length > 100) {
            feed.removeChild(feed.lastChild);
        }
    }

    function bindLiveQueryStream() {
        var panel = document.getElementById('live-query-stream');
        if (!panel) {
            closeLiveQueryStream();
            return;
        }
        if (panel.dataset.bound === 'true') {
            return;
        }
        panel.dataset.bound = 'true';

        if (typeof window.EventSource === 'undefined') {
            return;
        }

        var domainInput = document.getElementById('live-query-domain-filter');
        var categorySelect = document.getElementById('live-query-category-filter');
        var feed = document.getElementById('live-query-feed');
        var clearButton = document.getElementById('live-query-clear');
        if (!domainInput || !categorySelect || !feed || !clearButton) {
            return;
        }

        var debounceTimer = null;

        function openStream() {
            var actor = panel.dataset.actorKey || '';
            var domain = domainInput.value.trim();
            var category = categorySelect.value;
            var params = new URLSearchParams();

            if (actor) {
                params.set('actor', actor);
            }
            if (domain) {
                params.set('domain', domain);
            }
            if (category) {
                params.set('category', category);
            }

            var signature = [actor, domain, category].join('|');
            if (liveQueryStream.source && liveQueryStream.signature === signature) {
                return;
            }

            closeLiveQueryStream();
            liveQueryStream.signature = signature;

            var url = '/api/queries/stream';
            var queryString = params.toString();
            if (queryString) {
                url += '?' + queryString;
            }

            liveQueryStream.source = new EventSource(url);
            liveQueryStream.source.addEventListener('query', function (event) {
                var query;
                try {
                    query = JSON.parse(event.data);
                } catch (_) {
                    return;
                }
                appendLiveQueryRow(feed, query);
            });
        }

        domainInput.addEventListener('input', function () {
            if (debounceTimer) {
                clearTimeout(debounceTimer);
            }
            debounceTimer = setTimeout(openStream, 300);
        });
        categorySelect.addEventListener('change', openStream);
        clearButton.addEventListener('click', function () {
            domainInput.value = '';
            categorySelect.value = '';
            feed.dataset.hasRows = 'false';
            feed.innerHTML = '<tr><td colspan="5">Waiting for live queries…</td></tr>';
            openStream();
        });

        openStream();
    }

    function initPrivacyUI(root) {
        bindDeviceList();
        bindLiveQueryStream();
        (root || document).querySelectorAll('.privacy-chart').forEach(bindChart);
    }

    document.addEventListener('DOMContentLoaded', function () {
        initPrivacyUI(document);
    });
    document.body.addEventListener('htmx:afterSwap', function (event) {
        initPrivacyUI(event.target);
    });
})();
