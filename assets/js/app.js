document.addEventListener('alpine:init', () => {
  Alpine.data('receiptsPage', () => ({
    providers: [],
    receipts: [],
    selectedProviders: [],
    selectedPeriod: '',
    query: '',
    loading: false,
    // 'all' | 'paid' | 'unpaid'
    paidFilter: 'all',
    paidFilters: [
      { value: 'all', label: t('Sve') },
      { value: 'paid', label: t('Plaćeno') },
      { value: 'unpaid', label: t('Neplaćeno') },
    ],
    // URLs of receipts with an in-flight paid toggle, so their button can show
    // progress and cannot be double-submitted.
    saving: [],
    // Pending pulse-reset timers, keyed by receipt URL.
    pulseTimers: {},

    /**
     * Every period present in the data, newest first.
     *
     * A plain string sort would put "12-2025" ahead of "07-2026" because the
     * month comes first in "MM-YYYY". Compare by year then month instead.
     *
     * @returns {string[]}
     */
    get periods() {
      return Array.from(new Set(this.receipts.map(r => r.period)))
        .sort((a, b) => this.periodKey(b) - this.periodKey(a));
    },

    /**
     * @param {string} period
     *
     * @returns {number}
     */
    periodKey(period) {
      const [month, year] = String(period || '').split('-').map(Number);
      if (!month || !year) {
        return 0;
      }

      return year * 100 + month;
    },

    /**
     * @returns {boolean}
     */
    get hasActiveFilters() {
      return this.selectedProviders.length > 0
        || this.selectedPeriod !== ''
        || this.paidFilter !== 'all'
        || this.query.trim() !== '';
    },

    /**
     * Whether the provider itself reports the receipt as paid, from the
     * database status column ("plaćeno"). NFC-normalized so the comparison does
     * not depend on how the accented characters were encoded.
     *
     * @param {object} item
     * @returns {boolean}
     */
    isProviderPaid(item) {
      return (item.status || '').normalize('NFC') === 'plaćeno'.normalize('NFC');
    },

    /**
     * Effective paid state: either the provider reports it paid, or the user
     * marked it paid here.
     *
     * @param {object} item
     * @returns {boolean}
     */
    isPaid(item) {
      return !!item.paid || this.isProviderPaid(item);
    },

    /**
     * Whether a receipt's payment is verified by the provider. The user marking
     * it paid is only their own claim (they scanned the QR and paid); the
     * verified badge appears once a later fetch shows the provider itself
     * reports the receipt as paid.
     *
     * @param {object} item
     * @returns {boolean}
     */
    isVerified(item) {
      return this.isProviderPaid(item);
    },

    /**
     * Unpaid receipts across the whole set, not just the filtered view, so the
     * number does not change as filters are applied.
     *
     * @returns {number}
     */
    get unpaidCount() {
      return this.receipts.filter(r => !this.isPaid(r)).length;
    },

    /**
     * The receipt list after applying every active filter.
     *
     * Filtering is done entirely client side: the full list is fetched once so
     * that the provider and period pickers always show all available options,
     * regardless of what is currently selected.
     *
     * @returns {object[]}
     */
    get filteredReceipts() {
      let items = this.receipts.slice();

      if (this.selectedProviders.length > 0) {
        const allowed = new Set(this.selectedProviders.map(p => p.toLowerCase()));
        items = items.filter(r => allowed.has((r.provider || '').toLowerCase()));
      }

      if (this.selectedPeriod) {
        items = items.filter(r => r.period === this.selectedPeriod);
      }

      if (this.paidFilter !== 'all') {
        const wantPaid = this.paidFilter === 'paid';
        items = items.filter(r => this.isPaid(r) === wantPaid);
      }

      const q = this.query.trim().toLowerCase();
      if (q) {
        items = items.filter(r =>
          (r.provider || '').toLowerCase().includes(q) ||
          (r.filename || '').toLowerCase().includes(q) ||
          (r.period || '').toLowerCase().includes(q)
        );
      }

      // Newest first, then alphabetically by provider.
      items.sort((a, b) =>
        (b.modified || 0) - (a.modified || 0) ||
        (a.provider || '').localeCompare(b.provider || '')
      );

      return items;
    },

    async init() {
      this.loading = true;
      try {
        await Promise.all([this.fetchProviders(), this.fetchReceipts()]);
      } finally {
        this.loading = false;
      }

      // The header's refresh control fires this once new receipts have landed.
      window.addEventListener('receipts-updated', () => this.fetchReceipts());
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

    async fetchReceipts() {
      try {
        const res = await fetch('/api/receipts');
        if (!res.ok) {
          throw new Error(`failed to fetch receipts: ${res.status}`);
        }

        const data = await res.json();
        this.receipts = Array.isArray(data) ? data : [];
      } catch (e) {
        console.error(e);
        this.receipts = [];
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
    },

    clearProviders() {
      this.selectedProviders = [];
    },

    resetFilters() {
      this.selectedProviders = [];
      this.selectedPeriod = '';
      this.paidFilter = 'all';
      this.query = '';
    },

    /**
     * Flips a receipt between paid and unpaid.
     *
     * The card updates optimistically and rolls back if the request fails, so
     * the indicator never claims a state the server did not accept.
     *
     * @param {object} item
     */
    async togglePaid(item) {
      if (this.saving.includes(item.url)) {
        return;
      }

      const previous = { paid: item.paid, paid_at: item.paid_at };
      const next = !item.paid;

      this.saving.push(item.url);
      item.paid = next;

      try {
        const res = await fetch(`/api/receipts/${item.period}/${item.provider}/paid`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ paid: next }),
        });

        if (!res.ok) {
          throw new Error(`failed to update paid state: ${res.status}`);
        }

        const json = await res.json();
        item.paid = !!json.paid;
        item.paid_at = Number(json.paid_at || 0);

        this.pulse(item);
      } catch (e) {
        console.error(e);
        item.paid = previous.paid;
        item.paid_at = previous.paid_at;
      } finally {
        this.saving = this.saving.filter(u => u !== item.url);
      }
    },

    /**
     * Flags a receipt so its status badge plays the pop animation once.
     *
     * The flag is cleared afterwards, otherwise the animation class would stay
     * on the element and never replay on the next toggle.
     *
     * @param {object} item
     */
    pulse(item) {
      clearTimeout(this.pulseTimers[item.url]);

      // Clear first, then set on the next task, so a rapid second toggle
      // removes and re-adds the class and the animation actually restarts.
      // A timer rather than requestAnimationFrame: rAF is paused entirely in a
      // background tab, which would leave the flag stuck off.
      item.pulse = false;

      setTimeout(() => {
        item.pulse = true;

        this.pulseTimers[item.url] = setTimeout(() => {
          item.pulse = false;
          delete this.pulseTimers[item.url];
        }, 450);
      }, 0);
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
     * Formats a unix timestamp (seconds) as DD.MM.YYYY, or '' when unset.
     *
     * @param {number} ts
     *
     * @returns {string}
     */
    formatDate(ts) {
      const num = Number(ts) || 0;
      if (num <= 0) {
        return '';
      }

      const d = new Date(num * 1000);
      const dd = String(d.getDate()).padStart(2, '0');
      const mm = String(d.getMonth() + 1).padStart(2, '0');

      return `${dd}.${mm}.${d.getFullYear()}`;
    },

    /**
     * @param {number} bytes
     *
     * @returns {string}
     */
    formatSize(bytes) {
      const num = Number(bytes) || 0;
      if (num < 1024) {
        return `${num} B`;
      }

      const units = ['KB', 'MB', 'GB'];
      let value = num / 1024;
      let unit = 0;

      while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
      }

      return `${value.toFixed(1)} ${units[unit]}`;
    },
  }));
});
