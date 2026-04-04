(function () {
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

    function bindAllCharts(root) {
        (root || document).querySelectorAll('.privacy-chart').forEach(bindChart);
    }

    // Resize observer for responsive chart containers
    if (typeof ResizeObserver !== 'undefined') {
        var resizeObserver = new ResizeObserver(function (entries) {
            entries.forEach(function (entry) {
                var container = entry.target;
                if (container._uplot && container.clientWidth > 0) {
                    container._uplot.setSize({ width: container.clientWidth, height: 280 });
                }
            });
        });

        var originalBindChart = bindChart;
        bindChart = function (container) {
            originalBindChart(container);
            if (container && container._uplot) {
                resizeObserver.observe(container);
            }
        };
    }

    document.addEventListener('DOMContentLoaded', function () {
        bindAllCharts(document);
    });
    document.body.addEventListener('htmx:afterSwap', function (event) {
        bindAllCharts(event.target);
    });
})();
