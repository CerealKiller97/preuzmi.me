document.addEventListener('alpine:init', () => {
  Alpine.data('receipts', () => ({
    providers: [],
    receipts: [],
    selectedProviders: [],
    selectedPeriod: '',
    query: '',
    loading: false,

    get periods() {
      const uniq = new Set(this.receipts.map(r => r.period));
      return Array.from(uniq).sort().reverse();
    },

    get filteredReceipts() {
      let items = this.receipts.slice();

      // Provider filter
      if (this.selectedProviders.length > 0) {
        const allowed = new Set(this.selectedProviders.map(p => (p || '').toLowerCase()));
        items = items.filter(r => allowed.has((r.provider || '').toLowerCase()));
      }

      // Period filter
      if (this.selectedPeriod) {
        items = items.filter(r => (r.period || '') === this.selectedPeriod);
      }

      // Text query filter
      const q = (this.query || '').trim().toLowerCase();
      if (q.length > 0) {
        items = items.filter(r =>
          (r.provider || '').toLowerCase().includes(q) ||
          (r.filename || '').toLowerCase().includes(q) ||
          (r.period || '').toLowerCase().includes(q)
        );
      }

      // Sort: newest modified first, then by provider
      items.sort((a, b) => (b.modified || 0) - (a.modified || 0) || (a.provider || '').localeCompare(b.provider || ''));

      return items;
    },

    async init() {
      await this.fetchProviders();
      await this.fetchReceipts();
    },

    async fetchProviders() {
      try {
        const res = await fetch('/api/providers');
        if (!res.ok) throw new Error('failed to fetch providers');
        const data = await res.json();
        this.providers = Array.isArray(data) ? data : [];
      } catch (e) {
        console.error(e);
        this.providers = [];
      }
    },

    async fetchReceipts() {
      try {
        this.loading = true;
        // Build query from current filters (optional: could use server-side filtering)
        const params = new URLSearchParams();
        if (this.selectedProviders.length > 0) {
            params.set('provider', this.selectedProviders.join(','));
        }

        if (this.selectedPeriod) {
            params.set('period', this.selectedPeriod);
        }

        const url = '/api/receipts' + (params.toString() ? `?${params.toString()}` : '');

        const res = await fetch(url);
        if (!res.ok) {
            throw new Error('failed to fetch receipts');
        }

        const data = await res.json();
        this.receipts = Array.isArray(data) ? data : [];
      } catch (e) {
        console.error(e);
        this.receipts = [];
      } finally {
        this.loading = false;
      }
    },

    toggleProvider(p) {
      const idx = this.selectedProviders.indexOf(p);
      if (idx >= 0) {
        this.selectedProviders.splice(idx, 1);
      } else {
        this.selectedProviders.push(p);
      }
      // Optionally refetch from server using query params
      this.fetchReceipts();
    },

    clearProviders() {
      this.selectedProviders = [];
      this.fetchReceipts();
    }
  }));
});
