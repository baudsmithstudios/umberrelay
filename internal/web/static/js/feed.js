(function () {
    const liveQueryStream = {
        source: null,
        signature: '',
    };

    function closeLiveQueryStream() {
        if (liveQueryStream.source) {
            liveQueryStream.source.close();
            liveQueryStream.source = null;
        }
        liveQueryStream.signature = '';
        const status = document.getElementById('live-query-status');
        if (status) {
            status.innerHTML = '';
        }
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
        const ts = Number(unixSeconds);
        if (!Number.isFinite(ts)) {
            return 'now';
        }
        const date = new Date(ts * 1000);
        if (Number.isNaN(date.getTime())) {
            return 'now';
        }
        return date.toLocaleTimeString();
    }

    // Mirrors categoryTone in internal/web/activity_view.go.
    function categoryTone(category) {
        switch (category) {
        case 'tracking':
        case 'malware':
            return 'error';
        case 'advertising':
            return 'warn';
        case 'analytics':
        case 'telemetry':
            return 'success';
        default:
            return '';
        }
    }

    function appendLiveQueryRow(feed, query, showActor) {
        if (feed.dataset.hasRows !== 'true') {
            feed.innerHTML = '';
            feed.dataset.hasRows = 'true';
        }

        const category = query.category || 'unclassified';
        const tone = categoryTone(query.category);
        const row = document.createElement('li');
        row.className = 'elum-feed-item';
        if (tone) {
            row.dataset.tone = tone;
        }
        row.innerHTML =
            '<span class="elum-badge"' + (tone ? ' data-tone="' + tone + '"' : '') + '>[' + escapeHTML(category.toUpperCase()) + ']</span>' +
            '<span>' + escapeHTML(query.domain) + ' <span class="elum-card-subtitle">' + escapeHTML(query.query_type || '') + '</span>' +
            (showActor ? ' · ' + escapeHTML(formatLiveActor(query)) : '') + '</span>' +
            '<span class="elum-feed-meta">' + escapeHTML(formatLiveTime(query.timestamp)) + '</span>';
        feed.insertBefore(row, feed.firstChild);

        while (feed.children.length > 100) {
            feed.removeChild(feed.lastChild);
        }
    }

    function bindLiveQueryStream() {
        const panel = document.getElementById('live-query-stream');
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

        const domainInput = document.getElementById('live-query-domain-filter');
        const categorySelect = document.getElementById('live-query-category-filter');
        const feed = document.getElementById('live-query-feed');
        const clearButton = document.getElementById('live-query-clear');
        const status = document.getElementById('live-query-status');
        if (!domainInput || !categorySelect || !feed || !clearButton || !status) {
            return;
        }

        let debounceTimer = null;

        function openStream() {
            const actor = panel.dataset.actorKey || '';
            const domain = domainInput.value.trim();
            const category = categorySelect.value;
            const params = new URLSearchParams();

            if (actor) {
                params.set('actor', actor);
            }
            if (domain) {
                params.set('domain', domain);
            }
            if (category) {
                params.set('category', category);
            }

            const signature = [actor, domain, category].join('|');
            if (liveQueryStream.source && liveQueryStream.signature === signature) {
                return;
            }

            closeLiveQueryStream();
            liveQueryStream.signature = signature;

            let url = '/api/queries/stream';
            const queryString = params.toString();
            if (queryString) {
                url += '?' + queryString;
            }

            liveQueryStream.source = new EventSource(url);
            liveQueryStream.source.addEventListener('open', function () {
                status.innerHTML = '<span class="elum-spinner">streaming</span>';
            });
            liveQueryStream.source.addEventListener('error', function () {
                status.innerHTML = '';
            });
            liveQueryStream.source.addEventListener('query', function (event) {
                let query;
                try {
                    query = JSON.parse(event.data);
                } catch (_) {
                    return;
                }
                appendLiveQueryRow(feed, query, !actor);
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
            feed.innerHTML = '<li class="elum-feed-item">Waiting for live queries\u2026</li>';
            openStream();
        });

        // Stream only while the Live tab is showing; each open stream holds one of the
        // browser's few HTTP/1.1 connections to this host.
        const tabPanel = panel.closest('[role="tabpanel"]');
        function syncWithTab() {
            if (tabPanel && tabPanel.hidden) {
                closeLiveQueryStream();
            } else {
                openStream();
            }
        }
        if (tabPanel) {
            new MutationObserver(syncWithTab).observe(tabPanel, { attributes: true, attributeFilter: ['hidden'] });
        }
        syncWithTab();
    }

    document.addEventListener('DOMContentLoaded', bindLiveQueryStream);
    document.body.addEventListener('htmx:afterSwap', function () {
        bindLiveQueryStream();
    });
})();
