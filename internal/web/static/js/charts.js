(function () {
    function chartColor(name) {
        return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    }

    function formatTimeLabel(timestamp, range) {
        const d = new Date(timestamp * 1000);
        if (range === '24h') {
            return d.getHours().toString().padStart(2, '0') + ':00';
        }
        const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
        return months[d.getMonth()] + ' ' + d.getDate();
    }

    function snapPixel(value, dpr) {
        return Math.round(value * dpr) / dpr;
    }

    function niceMax(value, steps) {
        if (!isFinite(value) || value <= 0) {
            return 1;
        }
        const roughStep = value / steps;
        const magnitude = Math.pow(10, Math.floor(Math.log10(roughStep)));
        const normalized = roughStep / magnitude;
        let step = 1;
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
        for (let i = 1; i < points.length; i++) {
            ctx.lineTo(points[i].x, points[i].y);
        }
        ctx.stroke();

        ctx.fillStyle = color;
        for (let i = 0; i < points.length; i++) {
            ctx.beginPath();
            ctx.arc(points[i].x, points[i].y, 2, 0, Math.PI * 2);
            ctx.fill();
        }
    }

    function drawChart(canvas, datasets, xlabels) {
        const dpr = window.devicePixelRatio || 1;
        const rect = canvas.parentElement.getBoundingClientRect();
        canvas.width = Math.max(1, Math.round(rect.width * dpr));
        canvas.height = Math.max(1, Math.round(rect.height * dpr));

        const ctx = canvas.getContext('2d');
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        const W = canvas.width / dpr;
        const H = canvas.height / dpr;
        ctx.clearRect(0, 0, W, H);

        const pad = { top: 26, right: 44, bottom: 30, left: 44 };
        const plotW = W - pad.left - pad.right;
        const plotH = H - pad.top - pad.bottom;

        const borderColor = chartColor('--umberrelay-panel-border') || '#5a4528';
        const gridColor = 'rgba(90, 69, 40, 0.25)';
        const mutedColor = chartColor('--pico-muted-color') || '#b09470';

        const primaryVals = datasets[0] ? datasets[0].values : [];
        const secondaryVals = datasets[1] ? datasets[1].values : [];
        const gridSteps = 4;
        const maxPrimary = niceMax(Math.max.apply(null, primaryVals), gridSteps);
        const maxSecondary = niceMax(Math.max.apply(null, secondaryVals), gridSteps);
        const minVal = 0;

        ctx.font = '11px SFMono-Regular, Cascadia Code, Consolas, monospace';
        ctx.fillStyle = mutedColor;
        ctx.textBaseline = 'middle';

        for (let g = 0; g <= gridSteps; g++) {
            const ratio = g / gridSteps;
            const y = snapPixel(pad.top + plotH - (ratio * plotH), dpr);

            ctx.strokeStyle = gridColor;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(pad.left, y);
            ctx.lineTo(pad.left + plotW, y);
            ctx.stroke();

            const leftLabel = Math.round(minVal + ratio * maxPrimary);
            ctx.textAlign = 'right';
            ctx.fillText(leftLabel.toString(), pad.left - 6, y);

            const rightLabel = Math.round(minVal + ratio * maxSecondary);
            ctx.textAlign = 'left';
            ctx.fillText(rightLabel.toString(), pad.left + plotW + 6, y);
        }

        ctx.textBaseline = 'top';
        ctx.textAlign = 'center';
        if (xlabels && xlabels.length > 0) {
            const step = Math.max(1, Math.floor(xlabels.length / 7));
            for (let xi = 0; xi < xlabels.length; xi += step) {
                const lx = snapPixel(pad.left + (xi / Math.max(1, xlabels.length - 1)) * plotW, dpr);
                ctx.fillText(xlabels[xi], lx, snapPixel(pad.top + plotH + 8, dpr));
            }
        }

        for (let di = 0; di < datasets.length; di++) {
            const ds = datasets[di];
            const vals = ds.values;
            const n = vals.length;
            if (n === 0) {
                continue;
            }

            let seriesMax = maxPrimary;
            if (ds.axis === 'right') {
                seriesMax = maxSecondary;
            }

            const points = [];
            for (let i = 0; i < n; i++) {
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
        const legendY = pad.top - 16;
        const swatchSize = 8;
        const swatchGap = 5;
        const itemGap = 14;
        let legendWidth = 0;
        for (let li = 0; li < datasets.length; li++) {
            legendWidth += swatchSize + swatchGap + ctx.measureText(datasets[li].label).width;
            if (li < datasets.length - 1) {
                legendWidth += itemGap;
            }
        }
        let legendX = pad.left + plotW - legendWidth;
        for (let li = 0; li < datasets.length; li++) {
            const item = datasets[li];
            ctx.fillStyle = item.color;
            ctx.fillRect(legendX, legendY - swatchSize / 2, swatchSize, swatchSize);
            legendX += swatchSize + swatchGap;
            ctx.fillStyle = mutedColor;
            ctx.fillText(item.label, legendX, legendY);
            legendX += ctx.measureText(item.label).width + itemGap;
        }
    }

    function renderChart(container, range) {
        const query = container.dataset.chartQuery || '';
        let url = '/api/activity?range=' + encodeURIComponent(range);
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
                const totals = buckets.map(function (bucket) { return bucket.total; });
                const trackerVals = buckets.map(function (bucket) { return bucket.tracker; });
                const xlabels = buckets.map(function (bucket) {
                    return formatTimeLabel(bucket.timestamp, range);
                });

                container.innerHTML = '';
                const canvas = document.createElement('canvas');
                canvas.style.display = 'block';
                canvas.style.width = '100%';
                canvas.style.height = '100%';
                container.appendChild(canvas);
                container._canvas = canvas;
                container._range = range;

                const totalColor = chartColor('--umberrelay-chart-total') || '#f5a623';
                const trackerColor = chartColor('--umberrelay-chart-tracker') || '#ff4f4f';

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

        const controls = document.querySelector('.range-controls[data-chart-id="' + container.id + '"]');
        let defaultRange = '7d';
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
        const resizeObserver = new ResizeObserver(function (entries) {
            entries.forEach(function (entry) {
                const container = entry.target;
                if (container._canvas && container.clientWidth > 0) {
                    const range = container._range || '7d';
                    renderChart(container, range);
                }
            });
        });

        const originalBindChart = bindChart;
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
