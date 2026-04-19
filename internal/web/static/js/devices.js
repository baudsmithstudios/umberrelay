(function () {
    const cardRefreshState = {
        source: null,
        refreshTimer: null,
        inFlight: false,
        pending: false,
    };

    function closeDeviceStream() {
        if (cardRefreshState.source) {
            cardRefreshState.source.close();
            cardRefreshState.source = null;
        }
    }

    function clampTrackerPercent(value) {
        const number = Number(value);
        if (!Number.isFinite(number)) {
            return 0;
        }
        if (number < 0) {
            return 0;
        }
        if (number > 100) {
            return 100;
        }
        return number;
    }

    function updateCardFromActor(card, actor) {
        const trackerPercent = clampTrackerPercent(actor.tracker_percent);
        const queryCount = Math.max(0, Number(actor.query_count || 0));
        card.dataset.trackerPercent = trackerPercent.toFixed(2);
        card.dataset.queryCount = String(queryCount);

        if (actor.name) {
            card.dataset.deviceName = actor.name;
            const nameNode = card.querySelector('.device-card-name');
            if (nameNode) {
                nameNode.textContent = actor.name;
            }
        }

        const badge = card.querySelector('.device-pct-badge');
        if (badge) {
            badge.textContent = Math.round(trackerPercent) + '%';
        }

        const bar = card.querySelector('.device-card-bar span');
        if (bar) {
            bar.style.width = trackerPercent.toFixed(2) + '%';
        }

        const queries = card.querySelector('.device-card-queries');
        if (queries) {
            queries.textContent = queryCount + (queryCount === 1 ? ' query' : ' queries');
        }
    }

    function buildCardMetaInfo(actor) {
        if (actor.type === 'source') {
            return 'Unattributed source';
        }
        return '';
    }

    function createDeviceCard(actor) {
        const card = document.createElement('a');
        card.href = '/devices/' + (actor.key || '');
        card.className = 'device-card';
        card.dataset.actorKey = actor.key || '';

        card.innerHTML =
            '<div class="device-card-header">' +
                '<strong class="device-card-name"></strong>' +
                '<span class="device-pct-badge">0%</span>' +
            '</div>' +
            '<div class="device-card-bar"><span style="width: 0%"></span></div>' +
            '<div class="device-card-meta">' +
                '<span class="device-card-info"></span>' +
                '<span class="device-card-queries">0 queries</span>' +
            '</div>' +
            '<div class="device-card-trends"></div>';

        const info = card.querySelector('.device-card-info');
        if (info) {
            info.textContent = buildCardMetaInfo(actor);
        }

        return card;
    }

    function refreshDeviceCards(list, applyListState) {
        if (cardRefreshState.inFlight) {
            cardRefreshState.pending = true;
            return;
        }
        cardRefreshState.inFlight = true;

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
                    if (actor && actor.key) {
                        byKey[actor.key] = actor;
                    }
                });

                const existingByKey = {};
                list.querySelectorAll('.device-card').forEach(function (card) {
                    const key = card.dataset.actorKey || '';
                    if (key) {
                        existingByKey[key] = card;
                    }
                    const actor = byKey[card.dataset.actorKey || ''];
                    if (actor) {
                        updateCardFromActor(card, actor);
                    }
                });

                actors.forEach(function (actor) {
                    if (!actor || !actor.key || existingByKey[actor.key]) {
                        return;
                    }
                    const card = createDeviceCard(actor);
                    updateCardFromActor(card, actor);
                    list.appendChild(card);
                });

                applyListState();
            })
            .catch(function () {})
            .finally(function () {
                cardRefreshState.inFlight = false;
                if (cardRefreshState.pending) {
                    cardRefreshState.pending = false;
                    refreshDeviceCards(list, applyListState);
                }
            });
    }

    function scheduleDeviceRefresh(list, applyListState, delayMs) {
        if (cardRefreshState.refreshTimer) {
            clearTimeout(cardRefreshState.refreshTimer);
        }
        cardRefreshState.refreshTimer = setTimeout(function () {
            refreshDeviceCards(list, applyListState);
        }, delayMs);
    }

    function bindDeviceStream(list, applyListState) {
        if (list.dataset.streamBound === 'true') {
            return;
        }
        list.dataset.streamBound = 'true';

        if (typeof window.EventSource === 'undefined') {
            return;
        }

        closeDeviceStream();
        cardRefreshState.source = new EventSource('/api/queries/stream?limit=1');
        cardRefreshState.source.addEventListener('query', function () {
            scheduleDeviceRefresh(list, applyListState, 250);
        });
        window.addEventListener('beforeunload', closeDeviceStream, { once: true });
    }

    function bindDeviceList() {
        const search = document.getElementById('device-search');
        const sort = document.getElementById('device-sort');
        const list = document.getElementById('device-list');
        if (!search || !sort || !list || list.dataset.bound === 'true') {
            return;
        }
        list.dataset.bound = 'true';

        function applyListState() {
            const query = search.value.toLowerCase();
            const cards = Array.prototype.slice.call(list.querySelectorAll('.device-card'));
            cards.forEach(function (card) {
                const name = (card.dataset.deviceName || '').toLowerCase();
                card.hidden = query !== '' && name.indexOf(query) === -1;
            });

            cards.sort(function (left, right) {
                if (sort.value === 'name') {
                    return (left.dataset.deviceName || '').localeCompare(right.dataset.deviceName || '');
                }
                if (sort.value === 'volume') {
                    return Number(right.dataset.queryCount || '0') - Number(left.dataset.queryCount || '0');
                }
                return Number(right.dataset.trackerPercent || '0') - Number(left.dataset.trackerPercent || '0');
            }).forEach(function (card) {
                list.appendChild(card);
            });
        }

        search.addEventListener('input', applyListState);
        sort.addEventListener('change', applyListState);
        applyListState();
        bindDeviceStream(list, applyListState);
    }

    document.addEventListener('DOMContentLoaded', bindDeviceList);
    document.body.addEventListener('htmx:afterSwap', function () {
        bindDeviceList();
    });
})();
