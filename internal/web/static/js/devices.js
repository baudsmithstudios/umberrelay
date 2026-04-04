(function () {
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
            var cards = Array.prototype.slice.call(list.querySelectorAll('.device-card'));
            cards.forEach(function (card) {
                var name = (card.dataset.deviceName || '').toLowerCase();
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
    }

    document.addEventListener('DOMContentLoaded', bindDeviceList);
    document.body.addEventListener('htmx:afterSwap', function () {
        bindDeviceList();
    });
})();
