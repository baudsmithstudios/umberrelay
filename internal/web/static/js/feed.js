(function () {
    var liveQueryStream = {
        source: null,
        signature: '',
    };

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
            feed.innerHTML = '<tr><td colspan="5">Waiting for live queries\u2026</td></tr>';
            openStream();
        });

        openStream();
    }

    document.addEventListener('DOMContentLoaded', bindLiveQueryStream);
    document.body.addEventListener('htmx:afterSwap', function () {
        bindLiveQueryStream();
    });
})();
