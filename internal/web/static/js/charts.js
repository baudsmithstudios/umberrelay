(function () {
    const PAD = { top: 28, right: 16, bottom: 28, left: 48 };

    function token(name) {
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

    function niceMax(value, steps) {
        const roughStep = value / steps;
        const magnitude = Math.pow(10, Math.floor(Math.log10(roughStep)));
        const normalized = roughStep / magnitude;
        let step = 10;
        if (normalized <= 1) {
            step = 1;
        } else if (normalized <= 2) {
            step = 2;
        } else if (normalized <= 5) {
            step = 5;
        }
        return Math.min(100, step * magnitude * steps);
    }

    function formatRate(value) {
        return (Math.round(value * 10) / 10) + '%';
    }

    // Rate per bucket; empty buckets are null so the line breaks instead of dropping to zero.
    function toSeries(buckets, range) {
        let total = 0;
        let tracker = 0;
        const points = buckets.map(function (bucket) {
            total += bucket.total;
            tracker += bucket.tracker;
            return {
                label: formatTimeLabel(bucket.timestamp, range),
                total: bucket.total,
                rate: bucket.total > 0 ? bucket.tracker / bucket.total * 100 : null,
            };
        });
        let peak = -1;
        points.forEach(function (p, i) {
            if (p.rate !== null && (peak < 0 || p.rate > points[peak].rate)) {
                peak = i;
            }
        });
        return { points: points, average: total > 0 ? tracker / total * 100 : null, peak: peak };
    }

    // Grid lines across the plot, labelled with their rate, and time labels along the bottom.
    function drawAxes(frame, points) {
        const ctx = frame.ctx;
        ctx.strokeStyle = frame.colors.grid;
        ctx.lineWidth = 1;
        ctx.fillStyle = frame.colors.muted;
        ctx.textAlign = 'right';
        ctx.textBaseline = 'middle';
        for (let step = 0; step <= 4; step++) {
            const value = frame.yMax / 4 * step;
            const py = Math.round(frame.y(value)) + 0.5;
            ctx.beginPath();
            ctx.moveTo(PAD.left, py);
            ctx.lineTo(PAD.left + frame.plotW, py);
            ctx.stroke();
            ctx.fillText(formatRate(value), PAD.left - 6, py);
        }

        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        const labelEvery = Math.max(1, Math.ceil(points.length / 6));
        points.forEach(function (p, i) {
            if (i % labelEvery === 0 || i === points.length - 1) {
                ctx.fillText(p.label, frame.x(i), PAD.top + frame.plotH + 8);
            }
        });
    }

    function drawAverage(frame, average) {
        const ctx = frame.ctx;
        const ay = Math.round(frame.y(average)) + 0.5;
        ctx.setLineDash([4, 4]);
        ctx.strokeStyle = frame.colors.muted;
        ctx.beginPath();
        ctx.moveTo(PAD.left, ay);
        ctx.lineTo(PAD.left + frame.plotW, ay);
        ctx.stroke();
        ctx.setLineDash([]);
        ctx.textAlign = 'right';
        ctx.textBaseline = 'bottom';
        ctx.fillText('avg ' + formatRate(average), PAD.left + frame.plotW, ay - 3);
    }

    function drawRateLine(frame, points) {
        const ctx = frame.ctx;
        ctx.strokeStyle = frame.colors.line;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';
        ctx.beginPath();
        let drawing = false;
        points.forEach(function (p, i) {
            if (p.rate === null) {
                drawing = false;
                return;
            }
            if (drawing) {
                ctx.lineTo(frame.x(i), frame.y(p.rate));
            } else {
                ctx.moveTo(frame.x(i), frame.y(p.rate));
                drawing = true;
            }
        });
        ctx.stroke();
    }

    // Marks the highest point and labels it, keeping the label inside the plot.
    function drawPeak(frame, points, index) {
        const ctx = frame.ctx;
        const peak = points[index];
        const px = frame.x(index);
        const py = frame.y(peak.rate);
        ctx.fillStyle = frame.colors.line;
        ctx.strokeStyle = frame.colors.bg;
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(px, py, 4, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
        const text = formatRate(peak.rate) + ' at ' + peak.label;
        const half = ctx.measureText(text).width / 2;
        ctx.fillStyle = frame.colors.fg;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'bottom';
        ctx.fillText(text, Math.min(Math.max(px, PAD.left + half), PAD.left + frame.plotW - half), py - 8);
    }

    function drawCursor(frame, index) {
        const ctx = frame.ctx;
        const cx = Math.round(frame.x(index)) + 0.5;
        ctx.strokeStyle = frame.colors.fg;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(cx, PAD.top);
        ctx.lineTo(cx, PAD.top + frame.plotH);
        ctx.stroke();
    }

    function draw(chart) {
        const canvas = chart.canvas;
        const series = chart.series;
        const dpr = window.devicePixelRatio || 1;
        const width = canvas.clientWidth;
        const height = canvas.clientHeight;
        canvas.width = Math.round(width * dpr);
        canvas.height = Math.round(height * dpr);
        const ctx = canvas.getContext('2d');
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, width, height);
        ctx.font = '11px ' + token('--elum-font-family');

        const plotW = width - PAD.left - PAD.right;
        const plotH = height - PAD.top - PAD.bottom;
        const points = series.points;
        const rates = points.map(function (p) { return p.rate; }).filter(function (r) { return r !== null; });
        const yMax = niceMax(Math.max.apply(null, rates.concat([series.average || 0, 1])), 4);
        const frame = {
            ctx: ctx,
            colors: {
                line: token('--elum-color-error'),
                muted: token('--elum-color-muted'),
                grid: token('--elum-color-border'),
                fg: token('--elum-color-fg'),
                bg: token('--elum-color-bg'),
            },
            plotW: plotW,
            plotH: plotH,
            yMax: yMax,
            x: function (i) { return PAD.left + (points.length > 1 ? i / (points.length - 1) * plotW : plotW / 2); },
            y: function (v) { return PAD.top + plotH - v / yMax * plotH; },
        };
        chart.x = frame.x;

        drawAxes(frame, points);
        if (series.average !== null) {
            drawAverage(frame, series.average);
        }
        drawRateLine(frame, points);
        if (series.peak >= 0) {
            drawPeak(frame, points, series.peak);
        }
        if (chart.active !== null) {
            drawCursor(frame, chart.active);
        }
        updateTooltip(chart);
    }

    function updateTooltip(chart) {
        const tip = chart.tooltip;
        if (chart.active === null) {
            tip.hidden = true;
            return;
        }
        const p = chart.series.points[chart.active];
        tip.textContent = p.label + ' · ' + (p.rate === null ? 'no queries' : formatRate(p.rate) + ' · ' + p.total + ' queries');
        tip.hidden = false;
        const left = Math.min(chart.x(chart.active) + 8, chart.canvas.clientWidth - tip.offsetWidth);
        tip.style.left = Math.max(0, left) + 'px';
    }

    function nearestIndex(chart, clientX) {
        const rect = chart.canvas.getBoundingClientRect();
        const offset = clientX - rect.left;
        let best = 0;
        chart.series.points.forEach(function (_, i) {
            if (Math.abs(chart.x(i) - offset) < Math.abs(chart.x(best) - offset)) {
                best = i;
            }
        });
        return best;
    }

    function load(chart, range) {
        const granularity = range === '24h' ? 'by hour' : 'by day';
        chart.subtitle.textContent = granularity;
        let url = '/api/activity?range=' + encodeURIComponent(range);
        if (chart.query) {
            url += '&' + chart.query;
        }
        fetch(url)
            .then(function (response) {
                if (!response.ok) {
                    throw new Error('request failed');
                }
                return response.json();
            })
            .then(function (buckets) {
                chart.series = toSeries(buckets, range);
                chart.active = null;
                chart.canvas.setAttribute('aria-label', 'Tracker rate ' + granularity +
                    (chart.series.average === null ? ', no queries' : ', average ' + formatRate(chart.series.average)) +
                    '. Use left and right arrows to read each point.');
                draw(chart);
            })
            // Clears the chart instead of replacing it, so the next range can still draw.
            .catch(function () {
                chart.subtitle.textContent = 'failed to load';
                chart.canvas.setAttribute('aria-label', 'Tracker rate failed to load.');
                chart.series = { points: [], average: null, peak: -1 };
                chart.active = null;
                draw(chart);
            });
    }

    function bind() {
        const container = document.getElementById('tracker-chart');
        container.style.position = 'relative';
        const canvas = document.createElement('canvas');
        canvas.tabIndex = 0;
        canvas.setAttribute('role', 'img');
        canvas.style.display = 'block';
        canvas.style.width = '100%';
        canvas.style.height = '15rem';
        const tooltip = document.createElement('div');
        tooltip.className = 'elum-badge';
        tooltip.setAttribute('aria-live', 'polite');
        tooltip.style.position = 'absolute';
        tooltip.style.top = '0';
        tooltip.style.background = token('--elum-color-bg');
        tooltip.hidden = true;
        container.append(canvas, tooltip);

        const chart = {
            canvas: canvas,
            tooltip: tooltip,
            subtitle: document.querySelector('[data-chart-subtitle]'),
            query: container.dataset.chartQuery || '',
            series: { points: [], average: null, peak: -1 },
            active: null,
        };

        canvas.addEventListener('pointermove', function (event) {
            if (chart.series.points.length) {
                chart.active = nearestIndex(chart, event.clientX);
                draw(chart);
            }
        });
        canvas.addEventListener('pointerleave', function () {
            chart.active = null;
            draw(chart);
        });
        canvas.addEventListener('keydown', function (event) {
            const last = chart.series.points.length - 1;
            if (last < 0 || (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')) {
                return;
            }
            event.preventDefault();
            const step = event.key === 'ArrowLeft' ? -1 : 1;
            chart.active = chart.active === null ? (step > 0 ? 0 : last) : Math.min(last, Math.max(0, chart.active + step));
            draw(chart);
        });
        canvas.addEventListener('blur', function () {
            chart.active = null;
            draw(chart);
        });

        document.querySelectorAll('input[name="chart-range"]').forEach(function (input) {
            input.addEventListener('change', function () {
                load(chart, input.value);
            });
        });
        if (typeof ResizeObserver !== 'undefined') {
            new ResizeObserver(function () {
                if (chart.series.points.length) {
                    draw(chart);
                }
            }).observe(container);
        }

        load(chart, document.querySelector('input[name="chart-range"]:checked').value);
    }

    document.addEventListener('DOMContentLoaded', bind);
})();
