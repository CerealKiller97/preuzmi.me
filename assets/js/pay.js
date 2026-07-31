document.addEventListener('alpine:init', () => {
  Alpine.data('payPage', () => ({
    receipts: [],
    selectedPeriod: '',
    loading: false,
    // URLs of receipts with an in-flight paid mark.
    saving: [],
    // Per-receipt QR load failures (keyed by item.url).
    qrMissing: {},
    // True after the first load so we only auto-pick a period once.
    periodPicked: false,

    /**
     * @param {string} period
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
     * Periods that still have at least one unpaid receipt, newest first.
     *
     * @returns {string[]}
     */
    get periods() {
      return Array.from(new Set(
        this.receipts.filter(r => !this.isPaid(r)).map(r => r.period)
      )).sort((a, b) => this.periodKey(b) - this.periodKey(a));
    },

    /**
     * @param {object} item
     * @returns {boolean}
     */
    isProviderPaid(item) {
      return (item.status || '').normalize('NFC') === 'plaćeno'.normalize('NFC');
    },

    /**
     * @param {object} item
     * @returns {boolean}
     */
    isPaid(item) {
      return !!item.paid || this.isProviderPaid(item);
    },

    /**
     * Unpaid receipts for the selected period (or every period when unset),
     * newest period first, then provider name.
     *
     * @returns {object[]}
     */
    get unpaid() {
      let items = this.receipts.filter(r => !this.isPaid(r));

      if (this.selectedPeriod) {
        items = items.filter(r => r.period === this.selectedPeriod);
      }

      items.sort((a, b) =>
        this.periodKey(b.period) - this.periodKey(a.period) ||
        (a.provider || '').localeCompare(b.provider || '')
      );

      return items;
    },

    /**
     * Sum of unpaid amounts still on the list (missing amounts count as 0).
     *
     * @returns {number}
     */
    get runningTotal() {
      return this.unpaid.reduce((sum, r) => sum + (Number(r.amount) || 0), 0);
    },

    /**
     * @param {object} item
     * @returns {string}
     */
    qrImgUrl(item) {
      return `${item.url}/qr.png`;
    },

    /**
     * @param {object} item
     * @returns {boolean}
     */
    isQRMissing(item) {
      return !!this.qrMissing[item.url];
    },

    /**
     * Reassign so Alpine sees the new key (nested mutation alone may not).
     *
     * @param {object} item
     */
    markQRMissing(item) {
      this.qrMissing = { ...this.qrMissing, [item.url]: true };
    },

    async init() {
      this.loading = true;
      try {
        await this.fetchReceipts();
        this.pickDefaultPeriod();
      } finally {
        this.loading = false;
      }

      window.addEventListener('receipts-updated', async () => {
        await this.fetchReceipts();
        // Keep the user's period choice; only re-pick when the list emptied.
        if (this.selectedPeriod && !this.periods.includes(this.selectedPeriod)) {
          this.pickDefaultPeriod();
        }
      });
    },

    /**
     * Default to the newest period that still has unpaid bills — the monthly
     * ritual — falling back to "all periods" when nothing is unpaid.
     */
    pickDefaultPeriod() {
      if (this.periodPicked && this.selectedPeriod) {
        return;
      }

      this.selectedPeriod = this.periods[0] || '';
      this.periodPicked = true;
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
     * One-tap paid mark. Unlike the dashboard toggle, this only ever marks
     * paid — undoing belongs on the receipts page.
     *
     * @param {object} item
     */
    async markPaid(item) {
      if (this.saving.includes(item.url) || this.isPaid(item)) {
        return;
      }

      const previous = { paid: item.paid, paid_at: item.paid_at };

      this.saving.push(item.url);
      item.paid = true;

      try {
        const res = await fetch(`/api/receipts/${item.period}/${item.provider}/paid`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ paid: true }),
        });

        if (!res.ok) {
          throw new Error(`failed to update paid state: ${res.status}`);
        }

        const json = await res.json();
        item.paid = !!json.paid;
        item.paid_at = Number(json.paid_at || 0);
      } catch (e) {
        console.error(e);
        item.paid = previous.paid;
        item.paid_at = previous.paid_at;
      } finally {
        this.saving = this.saving.filter(u => u !== item.url);
      }
    },

    /**
     * @param {number} amount
     * @param {string} currency
     * @returns {string}
     */
    formatMoney(amount, currency) {
      const num = Number(amount) || 0;
      const sym = currency || 'RSD';

      try {
        return new Intl.NumberFormat('sr-Latn-RS', {
          style: 'currency',
          currency: sym,
          maximumFractionDigits: 0,
        }).format(num);
      } catch (_) {
        return `${num.toFixed(2)} ${sym}`;
      }
    },
  }));
});
