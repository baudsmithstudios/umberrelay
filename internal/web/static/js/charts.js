(function () {
    function chartColor(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }

    function formatTimeLabel(timestamp, range) {
        var d = new Date(timestamp * 1000);
        if (range === '24h') {
            return d.getHours().toString().padStart(2, '0') + ':00';
        }
        var months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
        return months[d.getMonth()] + ' ' + d.getDate();
    }

    function snapPixel(value, dpr) {
        return Math.round(value * dpr) / dpr;
    }

    function niceMax(value, steps) {
        if (!isFinite(value) || value <= 0) {
            return 1;
        }
        var roughStep = value / steps;
        var magnitude = Math.pow(10, Math.floor(Math.log10(roughStep)));
        var normalized = roughStep / magnitude;
        var step = 1;
        if (normalized > 1 && normalized <= 2) {
            step = 2;
        } else if (normalized > 2 && normalized <= 5) {
            step = 5;
        } else if (normalized > 5) {
            step = 10;
        }
        return step * magnitude * steps;
    }

    function drawSeries(ctx, points, color) {
        if (points.length === 0) {
            return;
        }
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.moveTo(points[0].x, points[0].y);
        for (var i = 1; i < points.length; i++) {
            ctx.lineTo(points[i].x, points[i].y);
        }
        ctx.stroke();

        ctx.fillStyle = color;
        for (var i = 0; i < points.length; i++) {
            ctx.beginPath();
            ctx.arc(points[i].x, points[i].y, 2, 0, Math.PI * 2);
            ctx.fill();
        }
    }

    function drawChart(canvas, datasets, xlabels) {
        var dpr = window.devicePixelRatio || 1;
        var rect = canvas.parentElement.getBoundingClientRect();
        canvas.width = Math.max(1, Math.round(rect.width * dpr));
        canvas.height = Math.max(1, Math.round(rect.height * dpr));

        var ctx = canvas.getContext('2d');
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        var W = canvas.width / dpr;
        var H = canvas.height / dpr;
        ctx.clearRect(0, 0, W, H);

        var pad = { top: 26, right: 44, bottom: 30, left: 44 };
        var plotW = W - pad.left - pad.right;
        var plotH = H - pad.top - pad.bottom;

        var borderColor = chartColor('--umberrelay-panel-border') || '#5a4528';
        var gridColor = 'rgba(90, 69, 40, 0.25)';
        var mutedColor = chartColor('--pico-muted-color') || '#b09470';

        var primaryVals = datasets[0] ? datasets[0].values : [];
        var secondaryVals = datasets[1] ? datasets[1].values : [];
        var gridSteps = 4;
        var maxPrimary = niceMax(Math.max.apply(null, primaryVals), gridSteps);
        var maxSecondary = niceMax(Math.max.apply(null, secondaryVals), gridSteps);
        var minVal = 0;

        ctx.font = '11px SFMono-Regular, Cascadia Code, Consolas, monospace';
        ctx.fillStyle = mutedColor;
        ctx.textBaseline = 'middle';

        for (var g = 0; g <= gridSteps; g++) {
            var ratio = g / gridSteps;
            var y = snapPixel(pad.top + plotH - (ratio * plotH), dpr);

            ctx.strokeStyle = gridColor;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(pad.left, y);
            ctx.lineTo(pad.left + plotW, y);
            ctx.stroke();

            var leftLabel = Math.round(minVal + ratio * maxPrimary);
            ctx.textAlign = 'right';
            ctx.fillText(leftLabel.toString(), pad.left - 6, y);

            var rightLabel = Math.round(minVal + ratio * maxSecondary);
            ctx.textAlign = 'left';
            ctx.fillText(rightLabel.toString(), pad.left + plotW + 6, y);
        }

        ctx.textBaseline = 'top';
        ctx.textAlign = 'center';
        if (xlabels && xlabels.length > 0) {
            var step = Math.max(1, Math.floor(xlabels.length / 7));
            for (var xi = 0; xi < xlabels.length; xi += step) {
                var lx = snapPixel(pad.left + (xi / Math.max(1, xlabels.length - 1)) * plotW, dpr);
                ctx.fillText(xlabels[xi], lx, snapPixel(pad.top + plotH + 8, dpr));
            }
        }

        for (var di = 0; di < datasets.length; di++) {
            var ds = datasets[di];
            var vals = ds.values;
            var n = vals.length;
            if (n === 0) {
                continue;
            }

            var seriesMax = maxPrimary;
            if (ds.axis === 'right') {
                seriesMax = maxSecondary;
            }

            var points = [];
            for (var i = 0; i < n; i++) {
                points.push({
                    x: pad.left + (i / Math.max(1, n - 1)) * plotW,
                    y: pad.top + plotH - ((vals[i] - minVal) / seriesMax) * plotH,
                });
            }

            drawSeries(ctx, points, ds.color);
        }

        ctx.strokeStyle = borderColor;
        ctx.lineWidth = 1;
        ctx.strokeRect(pad.left, pad.top, plotW, plotH);

        ctx.font = '10px SFMono-Regular, Cascadia Code, Consolas, monospace';
        ctx.textBaseline = 'middle';
        ctx.textAlign = 'left';
        var legendY = pad.top - 16;
        var swatchSize = 8;
        var swatchGap = 5;
        var itemGap = 14;
        var legendWidth = 0;
        for (var li = 0; li < datasets.length; li++) {
            legendWidth += swatchSize + swatchGap + ctx.measureText(datasets[li].label).width;
            if (li < datasets.length - 1) {
                legendWidth += itemGap;
            }
        }
        var legendX = pad.left + plotW - legendWidth;
        for (var li = 0; li < datasets.length; li++) {
            var item = datasets[li];
            ctx.fillStyle = item.color;
            ctx.fillRect(legendX, legendY - swatchSize / 2, swatchSize, swatchSize);
            legendX += swatchSize + swatchGap;
            ctx.fillStyle = mutedColor;
            ctx.fillText(item.label, legendX, legendY);
            legendX += ctx.measureText(item.label).width + itemGap;
        }
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
                var totals = buckets.map(function (bucket) { return bucket.total; });
                var trackerVals = buckets.map(function (bucket) { return bucket.tracker; });
                var xlabels = buckets.map(function (bucket) {
                    return formatTimeLabel(bucket.timestamp, range);
                });

                container.innerHTML = '';
                var canvas = document.createElement('canvas');
                canvas.style.display = 'block';
                canvas.style.width = '100%';
                canvas.style.height = '100%';
                container.appendChild(canvas);
                container._canvas = canvas;
                container._range = range;

                var totalColor = chartColor('--umberrelay-chart-total') || '#f5a623';
                var trackerColor = chartColor('--umberrelay-chart-tracker') || '#ff4f4f';

                drawChart(canvas, [
                    { color: totalColor, values: totals, label: 'Queries', axis: 'left' },
                    { color: trackerColor, values: trackerVals, label: 'Tracker', axis: 'right' },
                ], xlabels);
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

    if (typeof ResizeObserver !== 'undefined') {
        var resizeObserver = new ResizeObserver(function (entries) {
            entries.forEach(function (entry) {
                var container = entry.target;
                if (container._canvas && container.clientWidth > 0) {
                    var range = container._range || '7d';
                    renderChart(container, range);
                }
            });
        });

        var originalBindChart = bindChart;
        bindChart = function (container) {
            originalBindChart(container);
            if (container) {
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
