(function () {
    function chartColor(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }

    function formatTimeLabel(timestamp, range) {
        var d = new Date(timestamp * 1000);
        if (range === '24h') {
            return d.getHours().toString().padStart(2, '0') + ':00';
        }
        var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
        return months[d.getMonth()] + ' ' + d.getDate();
    }

    function drawRetroChart(canvas, datasets, xlabels) {
        var dpr = window.devicePixelRatio || 1;
        var rect = canvas.parentElement.getBoundingClientRect();
        canvas.width = rect.width * dpr;
        canvas.height = rect.height * dpr;
        var ctx = canvas.getContext('2d');
        ctx.scale(dpr, dpr);
        var W = rect.width;
        var H = rect.height;

        var pad = { top: 16, right: 12, bottom: 26, left: 44 };
        var plotW = W - pad.left - pad.right;
        var plotH = H - pad.top - pad.bottom;

        var bgColor = chartColor('--pico-background-color') || '#0e0b04';
        var borderColor = chartColor('--umberrelay-panel-border') || '#5a4528';
        var mutedColor = chartColor('--pico-muted-color') || '#b09470';
        var textColor = chartColor('--pico-color') || '#f0e6c8';

        // Background
        ctx.fillStyle = bgColor;
        ctx.fillRect(0, 0, W, H);

        // Scanline overlay
        ctx.fillStyle = 'rgba(240,230,200,0.015)';
        for (var sy = 0; sy < H; sy += 3) {
            ctx.fillRect(0, sy, W, 1);
        }

        // Value range across all datasets
        var allVals = [];
        for (var di = 0; di < datasets.length; di++) {
            allVals = allVals.concat(datasets[di].values);
        }
        var maxVal = Math.max.apply(null, allVals) * 1.15;
        if (maxVal === 0) { maxVal = 1; }
        var minVal = 0;

        // Grid lines
        ctx.strokeStyle = 'rgba(90,69,40,0.4)';
        ctx.lineWidth = 0.5;
        var gridSteps = 4;
        var font = '9px SFMono-Regular, Cascadia Code, Consolas, monospace';
        ctx.font = font;
        ctx.fillStyle = mutedColor;
        ctx.textAlign = 'right';
        for (var g = 0; g <= gridSteps; g++) {
            var gy = pad.top + plotH - (g / gridSteps) * plotH;
            ctx.beginPath();
            ctx.setLineDash([2, 3]);
            ctx.moveTo(pad.left, gy);
            ctx.lineTo(pad.left + plotW, gy);
            ctx.stroke();
            ctx.setLineDash([]);
            var label = Math.round(minVal + (g / gridSteps) * maxVal);
            ctx.fillText(label.toString(), pad.left - 6, gy + 3);
        }

        // X-axis labels
        ctx.textAlign = 'center';
        ctx.fillStyle = mutedColor;
        if (xlabels && xlabels.length > 0) {
            var step = Math.max(1, Math.floor(xlabels.length / 7));
            for (var xi = 0; xi < xlabels.length; xi += step) {
                var lx = pad.left + (xi / Math.max(1, xlabels.length - 1)) * plotW;
                ctx.fillText(xlabels[xi], lx, H - 6);
            }
        }

        // Draw each dataset
        for (var di = 0; di < datasets.length; di++) {
            var ds = datasets[di];
            var vals = ds.values;
            var n = vals.length;
            if (n === 0) { continue; }

            var points = [];
            for (var i = 0; i < n; i++) {
                points.push({
                    x: pad.left + (i / Math.max(1, n - 1)) * plotW,
                    y: pad.top + plotH - ((vals[i] - minVal) / maxVal) * plotH,
                });
            }

            // Glow effect
            ctx.save();
            ctx.shadowColor = ds.color;
            ctx.shadowBlur = 6;
            ctx.strokeStyle = ds.color;
            ctx.lineWidth = 1.5;
            ctx.lineJoin = 'round';
            ctx.lineCap = 'round';
            ctx.beginPath();
            ctx.moveTo(points[0].x, points[0].y);
            for (var i = 1; i < points.length; i++) {
                var prev = points[i - 1];
                var curr = points[i];
                var controlX = (prev.x + curr.x) / 2;
                ctx.bezierCurveTo(controlX, prev.y, controlX, curr.y, curr.x, curr.y);
            }
            ctx.stroke();
            ctx.restore();

            // Square dot markers
            ctx.fillStyle = ds.color;
            for (var i = 0; i < points.length; i++) {
                ctx.fillRect(points[i].x - 2, points[i].y - 2, 4, 4);
            }
        }

        // Plot area border
        ctx.strokeStyle = borderColor;
        ctx.lineWidth = 1;
        ctx.strokeRect(pad.left, pad.top, plotW, plotH);

        // Legend — bottom-right inside plot area
        ctx.font = font;
        ctx.textAlign = 'right';
        var legendX = pad.left + plotW - 8;
        var legendY = pad.top + 14;
        for (var di = 0; di < datasets.length; di++) {
            ctx.fillStyle = datasets[di].color;
            ctx.fillRect(legendX - 50, legendY + di * 14 - 6, 6, 6);
            ctx.fillText(datasets[di].label, legendX, legendY + di * 14);
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

                drawRetroChart(canvas, [
                    { color: totalColor, values: totals, label: 'Queries' },
                    { color: trackerColor, values: trackerVals, label: 'Tracker' },
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

    // Resize observer for responsive chart containers
    if (typeof ResizeObserver !== 'undefined') {
        var resizeObserver = new ResizeObserver(function (entries) {
            entries.forEach(function (entry) {
                var container = entry.target;
                if (container._canvas && container.clientWidth > 0) {
                    // Re-render on resize
                    var range = container._range || '7d';
                    renderChart(container, range);
                }
            });
        });

        var originalBindChart = bindChart;
        bindChart = function (container) {
            originalBindChart(container);
            if (container && container._canvas) {
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
