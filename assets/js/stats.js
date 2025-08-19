document.addEventListener('alpine:init', () => {
  Alpine.data('stats', () => ({
    providers: [],
    selectedProviders: [],
    years: [],
    selectedYear: new Date().getFullYear(),
    data: { monthly: Array(12).fill(0), monthly_counts: Array(12).fill(0), total: 0, average: 0, currency: '' },
    loading: false,

    async init() {
      await this.fetchProviders();
      await this.fetchYears();
      await this.fetchStats();
    },

    async fetchProviders() {
      try {
        const res = await fetch('/api/providers');
        if (!res.ok) throw new Error('failed providers');
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
        if (!res.ok) throw new Error('failed receipts');
        const items = await res.json();
        const yrs = new Set();
        for (const it of (Array.isArray(items) ? items : [])) {
          const parts = String(it.period || '').split('-');
          if (parts.length === 2) {
            const y = parseInt(parts[1], 10);
            if (!isNaN(y)) yrs.add(y);
          }
        }
        const arr = Array.from(yrs).sort((a, b) => b - a);
        this.years = arr.length ? arr : [this.selectedYear];
        if (!this.years.includes(this.selectedYear)) this.selectedYear = this.years[0];
      } catch (e) {
        console.error(e);
        this.years = [this.selectedYear];
      }
    },

    async fetchStats() {
      try {
        this.loading = true;
        const params = new URLSearchParams();
        if (this.selectedProviders.length > 0) params.set('provider', this.selectedProviders.join(','));
        if (this.selectedYear) params.set('year', String(this.selectedYear));
        const res = await fetch('/api/stats?' + params.toString());
        if (!res.ok) throw new Error('failed stats');
        const json = await res.json();
        this.data = {
          monthly: Array.isArray(json.monthly) ? json.monthly : Array(12).fill(0),
          monthly_counts: Array.isArray(json.monthly_counts) ? json.monthly_counts : Array(12).fill(0),
          total: Number(json.total || 0),
          average: Number(json.average || 0),
          currency: String(json.currency || ''),
        };
      } catch (e) {
        console.error(e);
        this.data = { monthly: Array(12).fill(0), monthly_counts: Array(12).fill(0), total: 0, average: 0, currency: '' };
      } finally {
        this.loading = false;
      }
    },

    toggleProvider(p) {
      const i = this.selectedProviders.indexOf(p);
      if (i >= 0) this.selectedProviders.splice(i, 1);
      else this.selectedProviders.push(p);
      this.fetchStats();
    },

    clearProviders() {
      this.selectedProviders = [];
      this.fetchStats();
    },

    get totalCount() {
      return (this.data.monthly_counts || []).reduce((a, b) => a + (Number(b) || 0), 0);
    },

    monthShort(idx) {
      const names = ['Jan','Feb','Mar','Apr','Maj','Jun','Jul','Avg','Sep','Okt','Nov','Dec'];
      return names[idx] || '';
    },

    barHeight(val) {
      const arr = this.data.monthly || [];
      const max = Math.max(0, ...arr);
      if (!max) return 0;
      const pct = (Number(val) || 0) / max * 100;
      return Math.max(2, Math.min(100, Math.round(pct)));
    },

    formatMoney(amount, currency) {
      const num = Number(amount) || 0;
      const sym = currency || 'RSD';
      try {
        return new Intl.NumberFormat('sr-RS', { style: 'currency', currency: sym }).format(num);
      } catch (_) {
        return `${num.toFixed(2)} ${sym}`;
      }
    },
  }));
});
