(function () {
    // Narrows the domain rows already on this page; pagination still pages the full set.
    function bindDomainFilter() {
        const form = document.getElementById('domain-filters');
        const rows = document.getElementById('domain-rows');
        if (!form || !rows) {
            return;
        }
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

    document.addEventListener('DOMContentLoaded', bindDomainFilter);
})();
