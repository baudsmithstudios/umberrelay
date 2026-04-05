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

                var borderColor = chartColor('--umberrelay-panel-border') || '#5a4528';
                var gridColor = 'rgba(90, 69, 40, 0.3)';
                var font = '11px monospace';

                var axisOpts = {
                    stroke: borderColor,
                    grid: {
                        show: true,
                        stroke: gridColor,
                        width: 1 / devicePixelRatio,
                        dash: [2, 3],
                    },
                    ticks: {
                        show: true,
                        stroke: borderColor,
                        width: 1 / devicePixelRatio,
                        size: 4,
                    },
                    font: font,
                    labelFont: font,
                    labelSize: 16,
                };

                container._uplot = new uPlot({
                    width: container.clientWidth || 720,
                    height: 240,
                    scales: {
                        x: { time: true },
                        rate: { auto: true },
                        volume: { auto: true },
                    },
                    legend: { show: true, live: false },
                    cursor: {
                        show: true,
                        x: true,
                        y: false,
                        points: { show: false },
                    },
                    axes: [
                        Object.assign({}, axisOpts),
                        Object.assign({ scale: 'rate', label: 'Tracker %' }, axisOpts),
                        Object.assign({ scale: 'volume', label: 'Queries', side: 1 }, axisOpts),
                    ],
                    series: [
                        {},
                        {
                            label: 'Tracker rate',
                            scale: 'rate',
                            stroke: 'transparent',
                            width: 0,
                            paths: function () { return null; },
                            points: {
                                show: true,
                                size: 4,
                                width: 0,
                                fill: chartColor('--umberrelay-chart-tracker'),
                                stroke: chartColor('--umberrelay-chart-tracker'),
                            },
                        },
                        {
                            label: 'Queries',
                            scale: 'volume',
                            stroke: 'transparent',
                            width: 0,
                            paths: function () { return null; },
                            points: {
                                show: true,
                                size: 4,
                                width: 0,
                                fill: chartColor('--umberrelay-chart-total'),
                                stroke: chartColor('--umberrelay-chart-total'),
                            },
                        },
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
                    container._uplot.setSize({ width: container.clientWidth, height: 240 });
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
