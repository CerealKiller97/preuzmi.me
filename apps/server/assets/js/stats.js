const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Maj', 'Jun', 'Jul', 'Avg', 'Sep', 'Okt', 'Nov', 'Dec'].map((m) => t(m));

/**
 * Reads a design token from assets/css/app.css so the chart follows the theme
 * instead of hardcoding a second, drifting palette.
 *
 * @param {string} name
 *
 * @returns {string}
 */
function token(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

document.addEventListener('alpine:init', () => {
  Alpine.data('statsPage', () => ({
    providers: [],
    selectedProviders: [],
    years: [],
    selectedYear: new Date().getFullYear(),
    data: {
      monthly: Array(12).fill(0),
      monthly_counts: Array(12).fill(0),
      by_provider: [],
      monthly_by_provider: [],
      total: 0,
      average: 0,
      currency: '',
    },
    loading: false,
    chart: null,
    donut: null,
    // 'bar' stacks the providers per month; 'line' shows each as a trend.
    chartType: 'bar',
    monthNames: MONTHS,

    async init() {
      await this.fetchProviders();
      await this.fetchYears();
      await this.fetchStats();

      // Repaint the chart when the user flips the theme.
      window.addEventListener('theme-changed', () => this.restyleChart());

      // Chart.js sizes against the laid-out box. Entrance animations (transform)
      // and x-show flips leave the first paint blank until something resizes.
      this.observeChartBoxes();
    },

    /**
     * Keep canvases in sync with their containers after layout / visibility
     * changes (staggered fade-in, x-show, window resize).
     */
    observeChartBoxes() {
      if (typeof ResizeObserver === 'undefined') {
        return;
      }

      const watch = (canvas, chartKey) => {
        if (!canvas?.parentElement) {
          return;
        }

        const ro = new ResizeObserver(() => {
          const chart = this[chartKey];
          if (chart) {
            chart.resize();
          }
        });
        ro.observe(canvas.parentElement);
      };

      this.$nextTick(() => {
        watch(this.$refs.monthlyLine, 'chart');
        watch(this.$refs.providerDonut, 'donut');
      });
    },

    /**
     * Paint after Alpine has applied x-show / bindings, then force a resize
     * once the entrance animation has released its transform.
     */
    schedulePaint() {
      this.$nextTick(() => {
        this.renderChart();
        this.renderDonut();

        // Two rAFs: one for style flush, one after paint. Then a short delay
        // past the fade-in-up duration (0.4s + stagger) so transform:none sticks.
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            this.chart?.resize();
            this.donut?.resize();
            window.setTimeout(() => {
              this.chart?.resize();
              this.donut?.resize();
            }, 500);
          });
        });
      });
    },

    /**
     * Provider totals as percentages of the yearly total, largest first.
     *
     * @returns {{provider: string, amount: number, share: number}[]}
     */
    get byProvider() {
      const rows = this.data.by_provider || [];
      const total = this.data.total || 0;

      return rows.map(r => ({
        provider: r.provider,
        amount: r.amount,
        share: total > 0 ? (r.amount / total) * 100 : 0,
      }));
    },

    /**
     * The month with the highest spend in the selected year.
     *
     * @returns {{name: string, amount: number}}
     */
    get peakMonth() {
      const monthly = this.data.monthly || [];
      let best = -1;

      monthly.forEach((amount, idx) => {
        if (amount > 0 && (best < 0 || amount > monthly[best])) {
          best = idx;
        }
      });

      if (best < 0) {
        return { name: '', amount: 0 };
      }

      return { name: MONTHS[best], amount: monthly[best] };
    },

    /**
     * @returns {number}
     */
    get totalCount() {
      return (this.data.monthly_counts || []).reduce((a, b) => a + (Number(b) || 0), 0);
    },

    /**
     * Per-provider monthly series, largest yearly spender first.
     *
     * @returns {{provider: string, monthly: number[]}[]}
     */
    get providerSeries() {
      return this.data.monthly_by_provider || [];
    },

    /**
     * Yearly total for one provider row of the table.
     *
     * @param {{monthly: number[]}} row
     *
     * @returns {number}
     */
    yearlyFor(row) {
      return (row.monthly || []).reduce((a, b) => a + (Number(b) || 0), 0);
    },

    async fetchProviders() {
      try {
        const res = await fetch('/api/providers');
        if (!res.ok) {
          throw new Error(`failed to fetch providers: ${res.status}`);
        }

        const data = await res.json();
        this.providers = Array.isArray(data) ? data : [];
      } catch (e) {
        console.error(e);
        this.providers = [];
      }
    },

    async fetchYears() {
      try {
        const res = await fetch('/api/receipts');
        if (!res.ok) {
          throw new Error(`failed to fetch receipts: ${res.status}`);
        }

        const items = await res.json();
        const years = new Set();

        for (const item of (Array.isArray(items) ? items : [])) {
          const parts = String(item.period || '').split('-');
          if (parts.length === 2) {
            const y = parseInt(parts[1], 10);
            if (!isNaN(y)) {
              years.add(y);
            }
          }
        }

        const sorted = Array.from(years).sort((a, b) => b - a);
        this.years = sorted.length ? sorted : [this.selectedYear];

        if (!this.years.includes(this.selectedYear)) {
          this.selectedYear = this.years[0];
        }
      } catch (e) {
        console.error(e);
        this.years = [this.selectedYear];
      }
    },

    async fetchStats() {
      this.loading = true;

      try {
        const params = new URLSearchParams();
        if (this.selectedProviders.length > 0) {
          params.set('provider', this.selectedProviders.join(','));
        }
        if (this.selectedYear) {
          params.set('year', String(this.selectedYear));
        }

        const res = await fetch(`/api/stats?${params.toString()}`);
        if (!res.ok) {
          throw new Error(`failed to fetch stats: ${res.status}`);
        }

        const json = await res.json();
        this.data = {
          monthly: Array.isArray(json.monthly) ? json.monthly : Array(12).fill(0),
          monthly_counts: Array.isArray(json.monthly_counts) ? json.monthly_counts : Array(12).fill(0),
          by_provider: Array.isArray(json.by_provider) ? json.by_provider : [],
          monthly_by_provider: Array.isArray(json.monthly_by_provider) ? json.monthly_by_provider : [],
          total: Number(json.total || 0),
          average: Number(json.average || 0),
          currency: String(json.currency || ''),
        };
      } catch (e) {
        console.error(e);
        this.data = {
          monthly: Array(12).fill(0),
          monthly_counts: Array(12).fill(0),
          by_provider: [],
          monthly_by_provider: [],
          total: 0,
          average: 0,
          currency: '',
        };
      } finally {
        this.loading = false;
        this.schedulePaint();
      }
    },

    /**
     * @param {'bar'|'line'} type
     */
    setChartType(type) {
      if (this.chartType === type) {
        return;
      }

      this.chartType = type;
      this.renderChart();
    },

    /**
     * The categorical palette for provider series, read from the theme.
     *
     * @returns {string[]}
     */
    seriesColors() {
      return [1, 2, 3, 4, 5]
        .map(i => token(`--chart-series-${i}`))
        .filter(Boolean);
    },

    /**
     * The colour for one provider's series: its brand colour when we have
     * one, otherwise a stable slot from the generic palette.
     *
     * @param {string} provider
     * @param {number} i the provider's index in the current ordering
     *
     * @returns {string}
     */
    providerColor(provider, i) {
      const brand = token(`--brand-${String(provider || '').toLowerCase()}`);
      if (brand) {
        return brand;
      }

      const palette = this.seriesColors();
      return palette[i % palette.length];
    },

    /**
     * @param {string} hex a #rrggbb colour
     * @param {number} alpha
     *
     * @returns {string}
     */
    rgba(hex, alpha) {
      const n = parseInt(hex.replace('#', ''), 16);
      if (isNaN(n)) {
        return hex;
      }

      return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`;
    },

    /**
     * Shared tooltip styling: a dark, rounded card that matches the UI instead
     * of Chart.js's stock look.
     */
    tooltipStyle() {
      return {
        backgroundColor: token('--chart-tooltip-bg'),
        titleColor: token('--chart-tooltip-fg'),
        bodyColor: token('--chart-tooltip-fg'),
        footerColor: token('--chart-tooltip-fg'),
        borderColor: token('--chart-tooltip-border'),
        borderWidth: 1,
        padding: 12,
        cornerRadius: 10,
        boxPadding: 4,
        usePointStyle: true,
      };
    },

    /**
     * (Re)draws the monthly chart from scratch: the number of datasets changes
     * with the provider filter and the colours change with the theme, and
     * Chart.js resolves both at construction time, so rebuilding is the
     * reliable way. Twelve points per series makes it cheap.
     */
    renderChart() {
      /** @type {HTMLCanvasElement} */
      const el = this.$refs.monthlyLine;
      if (!el || typeof Chart === 'undefined') {
        console.warn('Chart.js not loaded or canvas is missing.');
        return;
      }

      if (this.chart) {
        this.chart.destroy();
        this.chart = null;
      }

      const grid = token('--chart-grid');
      const ticks = token('--chart-tick');
      const isBar = this.chartType === 'bar';

      const datasets = isBar ? this.barDatasets() : this.lineDatasets(el);

      this.chart = new Chart(el.getContext('2d'), {
        type: isBar ? 'bar' : 'line',
        data: {
          labels: MONTHS,
          datasets,
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          interaction: { mode: 'index', intersect: false },
          plugins: {
            legend: {
              display: datasets.length > 1,
              labels: {
                color: ticks,
                usePointStyle: true,
                pointStyle: 'circle',
                boxWidth: 8,
                boxHeight: 8,
                padding: 16,
              },
            },
            tooltip: {
              ...this.tooltipStyle(),
              callbacks: {
                label: ctx => ` ${ctx.dataset.label}: ${this.formatMoney(ctx.parsed.y || 0, this.data.currency)}`,
                // Stacked bars lose the at-a-glance total, so the tooltip
                // carries it instead.
                footer: items => {
                  if (!isBar || items.length < 2) {
                    return '';
                  }

                  const sum = items.reduce((a, it) => a + (it.parsed.y || 0), 0);
                  return `Ukupno: ${this.formatMoney(sum, this.data.currency)}`;
                },
              },
            },
          },
          scales: {
            x: {
              stacked: isBar,
              border: { display: false },
              grid: { display: false },
              ticks: { color: ticks },
            },
            y: {
              stacked: isBar,
              beginAtZero: true,
              border: { display: false },
              grid: { color: grid },
              ticks: {
                color: ticks,
                maxTicksLimit: 6,
                callback: value => this.formatCompact(value),
              },
            },
          },
        },
      });
    },

    /**
     * One rounded, stacked bar segment per provider and month.
     */
    barDatasets() {
      const palette = this.seriesColors();

      // With no per-provider series (empty year) fall back to the total.
      if (this.providerSeries.length === 0) {
        return [{
          label: t('Ukupno'),
          data: this.data.monthly || Array(12).fill(0),
          backgroundColor: palette[0],
          borderRadius: 6,
          borderSkipped: false,
          maxBarThickness: 28,
        }];
      }

      return this.providerSeries.map((row, i) => {
        const color = this.providerColor(row.provider, i);

        return {
          label: providerLabel(row.provider),
          data: row.monthly || Array(12).fill(0),
          backgroundColor: color,
          hoverBackgroundColor: this.rgba(color, 0.85),
          // Only round the top of the stack so thin segments don't become pills.
          borderRadius: i === this.providerSeries.length - 1
            ? { topLeft: 6, topRight: 6, bottomLeft: 0, bottomRight: 0 }
            : 0,
          borderSkipped: false,
          barPercentage: 0.78,
          categoryPercentage: 0.86,
          maxBarThickness: 40,
        };
      });
    },

    /**
     * The total as a gradient-filled area with one thin trend line per
     * provider on top of it.
     *
     * @param {HTMLCanvasElement} el
     */
    lineDatasets(el) {
      const line = token('--chart-line');

      const gradient = el.getContext('2d').createLinearGradient(0, 0, 0, el.parentNode.clientHeight || 300);
      gradient.addColorStop(0, this.rgba(line, 0.14));
      gradient.addColorStop(1, this.rgba(line, 0));

      const datasets = [{
        label: t('Ukupno'),
        data: this.data.monthly || Array(12).fill(0),
        borderColor: line,
        backgroundColor: gradient,
        borderWidth: 2.5,
        tension: 0.4,
        pointRadius: 0,
        pointHoverRadius: 5,
        pointBackgroundColor: line,
        fill: true,
      }];

      this.providerSeries.forEach((row, i) => {
        const color = this.providerColor(row.provider, i);
        datasets.push({
          label: providerLabel(row.provider),
          data: row.monthly || Array(12).fill(0),
          borderColor: color,
          backgroundColor: color,
          borderWidth: 1.5,
          tension: 0.4,
          pointRadius: 0,
          pointHoverRadius: 4,
          pointBackgroundColor: color,
          fill: false,
        });
      });

      return datasets;
    },

    /**
     * The yearly provider share as a donut with the total in the middle.
     */
    renderDonut() {
      /** @type {HTMLCanvasElement} */
      const el = this.$refs.providerDonut;
      if (!el || typeof Chart === 'undefined') {
        return;
      }

      if (this.donut) {
        this.donut.destroy();
        this.donut = null;
      }

      const rows = this.data.by_provider || [];
      if (rows.length === 0) {
        return;
      }

      const self = this;

      // Painting the total inside the donut's hole turns dead space into the
      // most useful number on the card.
      const centerText = {
        id: 'centerText',
        afterDraw(chart) {
          const meta = chart.getDatasetMeta(0);
          if (!meta.data.length) {
            return;
          }

          // While a segment is hovered its tooltip sits over the donut hole, so
          // skip the center total to avoid the two overlapping.
          const active = chart.tooltip && chart.tooltip.getActiveElements
            ? chart.tooltip.getActiveElements()
            : [];
          if (active.length > 0) {
            return;
          }

          const { x, y } = meta.data[0];
          const { ctx } = chart;

          ctx.save();
          ctx.textAlign = 'center';
          ctx.textBaseline = 'middle';
          ctx.fillStyle = token('--chart-tick');
          ctx.font = '12px ui-sans-serif, system-ui, sans-serif';
          ctx.fillText(t('Ukupno'), x, y - 12);
          ctx.fillStyle = token('--chart-line');
          ctx.font = '600 15px ui-sans-serif, system-ui, sans-serif';
          ctx.fillText(self.formatMoney(self.data.total, self.data.currency), x, y + 10);
          ctx.restore();
        },
      };

      this.donut = new Chart(el.getContext('2d'), {
        type: 'doughnut',
        data: {
          labels: rows.map(r => providerLabel(r.provider)),
          datasets: [{
            data: rows.map(r => r.amount),
            backgroundColor: rows.map((r, i) => this.providerColor(r.provider, i)),
            borderWidth: 0,
            spacing: 3,
            borderRadius: 6,
            hoverOffset: 8,
          }],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          cutout: '72%',
          layout: { padding: 8 },
          plugins: {
            legend: { display: false },
            tooltip: {
              ...this.tooltipStyle(),
              callbacks: {
                label: ctx => {
                  const amount = ctx.parsed || 0;
                  const share = this.data.total > 0 ? (amount / this.data.total) * 100 : 0;
                  return ` ${this.formatMoney(amount, this.data.currency)} · ${share.toFixed(1)}%`;
                },
              },
            },
          },
        },
        plugins: [centerText],
      });
    },

    /**
     * Rebuilds both charts with the current theme's colours; the render
     * methods already tear down any existing instances.
     */
    restyleChart() {
      if (this.chart) {
        this.renderChart();
      }
      if (this.donut) {
        this.renderDonut();
      }
    },

    /**
     * @param {string} p
     */
    toggleProvider(p) {
      const idx = this.selectedProviders.indexOf(p);
      if (idx >= 0) {
        this.selectedProviders.splice(idx, 1);
      } else {
        this.selectedProviders.push(p);
      }

      this.fetchStats();
    },

    clearProviders() {
      this.selectedProviders = [];
      this.fetchStats();
    },

    /**
     * @param {number} amount
     * @param {string} currency
     *
     * @returns {string}
     */
    formatMoney(amount, currency) {
      const num = Number(amount) || 0;
      const sym = currency || 'RSD';

      try {
        return new Intl.NumberFormat(srLocale(), {
          style: 'currency',
          currency: sym,
          maximumFractionDigits: 0,
        }).format(num);
      } catch (_) {
        return `${num.toFixed(2)} ${sym}`;
      }
    },

    /**
     * Short axis labels, e.g. 12000 -> "12k".
     *
     * @param {number} value
     *
     * @returns {string}
     */
    formatCompact(value) {
      const num = Number(value) || 0;
      if (Math.abs(num) >= 1000) {
        return `${(num / 1000).toFixed(num % 1000 === 0 ? 0 : 1)}k`;
      }

      return String(num);
    },
  }));
});
