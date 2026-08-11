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
    // Receipt currently shown in the payment QR modal (null when closed).
    qrItem: null,
    qrOpen: false,
    // Parsed IPS fields for the open modal (null until /qr.txt loads).
    qrIPS: null,
    // Raw IPS payload cached for "Kopiraj IPS podatke".
    qrPayload: '',
    // True when the modal's QR image 404'd (bill carries no readable IPS QR).
    qrMissing: false,
    // Brief "copied" feedback after copying the IPS payload from the modal.
    qrCopied: false,
    qrCopiedTimer: null,

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
     * Whole calendar days from today to the receipt's due date. Negative when
     * overdue; null when the deadline is unknown.
     *
     * @param {object} item
     * @returns {number|null}
     */
    daysUntilDue(item) {
      const due = Number(item.due_at) || 0;
      if (due <= 0) {
        return null;
      }

      const dueDay = new Date(due * 1000);
      dueDay.setHours(0, 0, 0, 0);
      const today = new Date();
      today.setHours(0, 0, 0, 0);

      return Math.round((dueDay - today) / 86400000);
    },

    /**
     * @param {object} item
     * @returns {boolean}
     */
    isOverdue(item) {
      const days = this.daysUntilDue(item);
      return days !== null && days < 0;
    },

    /**
     * Short badge label for unpaid receipts with a known deadline.
     * Empty string hides the badge.
     *
     * @param {object} item
     * @returns {string}
     */
    dueLabel(item) {
      const days = this.daysUntilDue(item);
      if (days === null) {
        return '';
      }
      if (days < 0) {
        return 'Dospeo';
      }
      if (days === 0) {
        return 'Danas';
      }
      if (days === 1) {
        return 'Sutra';
      }
      if (days <= 3) {
        return `Za ${days} dana`;
      }

      return '';
    },

    /**
     * @param {object} item
     * @returns {string}
     */
    dueTitle(item) {
      const due = this.formatDate(item.due_at);
      if (!due) {
        return '';
      }
      if (this.isOverdue(item)) {
        return `Dospeo ${due}`;
      }

      return `Dospeće ${due}`;
    },

    /**
     * The ?account=<id> selector for a receipt, or "" for a solo account. Every
     * receipt sub-resource (qr.png, qr.txt, the paid endpoint) carries it so the
     * request targets the right family-member account.
     *
     * @param {object} item
     * @returns {string}
     */
    accountQuery(item) {
      return item && item.account ? `?account=${encodeURIComponent(item.account)}` : '';
    },

    /**
     * The receipt's base path without any query string. item.url is
     * "/receipt/{period}/{provider}" plus an optional "?account=…"; sub-resources
     * live one segment deeper, so the suffix must go before the query.
     *
     * @param {object} item
     * @returns {string}
     */
    receiptPath(item) {
      return String((item && item.url) || '').split('?')[0];
    },

    /**
     * URL of the rendered IPS payment QR for a receipt. The image lives one
     * segment deeper than the receipt path, with the account selector preserved.
     *
     * @param {object} item
     * @returns {string}
     */
    qrImgUrl(item) {
      return `${this.receiptPath(item)}/qr.png${this.accountQuery(item)}`;
    },

    /**
     * Short display label for a receipt's account ("Mama"), or "" for a solo
     * account. Prefers the configured label, falling back to the account id.
     *
     * @param {object} item
     * @returns {string}
     */
    accountName(item) {
      if (!item || !item.account) {
        return '';
      }
      return item.label || item.account;
    },

    /**
     * Opens the payment QR modal for a receipt and loads IPS payment fields.
     *
     * @param {object} item
     */
    openQR(item) {
      this.qrMissing = false;
      this.qrCopied = false;
      this.qrIPS = null;
      this.qrPayload = '';
      this.qrItem = item;
      this.qrOpen = true;
      document.documentElement.classList.add('overflow-hidden');
      this.loadIPS(item);
    },

    /**
     * Closes the payment QR modal and unlocks page scroll.
     */
    closeQR() {
      this.qrOpen = false;
      this.qrItem = null;
      this.qrIPS = null;
      this.qrPayload = '';
      this.qrMissing = false;
      this.qrCopied = false;
      if (this.qrCopiedTimer) {
        clearTimeout(this.qrCopiedTimer);
        this.qrCopiedTimer = null;
      }
      document.documentElement.classList.remove('overflow-hidden');
    },

    /**
     * Fetches and parses the NBS IPS payload for the open modal receipt.
     *
     * @param {object} item
     */
    async loadIPS(item) {
      if (!item) {
        return;
      }

      try {
        const res = await fetch(`${this.receiptPath(item)}/qr.txt${this.accountQuery(item)}`);
        if (!res.ok) {
          // Image @error may also flip this; keep both paths in sync.
          this.qrMissing = true;
          return;
        }
        const payload = await res.text();
        // Modal may have closed or switched receipts while the fetch was in flight.
        if (!this.qrOpen || this.qrItem !== item) {
          return;
        }
        this.qrPayload = payload;
        this.qrIPS = this.parseIPS(payload);

        // The QR amount is authoritative (it is what the bank app charges). The
        // server reconciles the stored price on this same request, so mirror it
        // onto the in-memory receipt too, letting the card update without a
        // reload. qrItem is the same object as the one in `receipts`.
        if (this.qrIPS.amount && this.qrItem && this.qrItem.amount !== this.qrIPS.amount) {
          this.qrItem.amount = this.qrIPS.amount;
        }
      } catch (e) {
        console.error('Load IPS payload failed', e);
      }
    },

    /**
     * Parses an NBS IPS QR payload into labelled fields for the modal.
     * Format: K:PR|V:01|C:1|R:account|N:name|I:RSD…|SF:code|S:purpose|RO:ref
     *
     * @param {string} payload
     * @returns {{recipient: string, account: string, reference: string, code: string, purpose: string, amount: number|null}}
     */
    parseIPS(payload) {
      const fields = {};
      String(payload || '').split('|').forEach(part => {
        const i = part.indexOf(':');
        if (i > 0) {
          fields[part.slice(0, i).toUpperCase()] = part.slice(i + 1).trim();
        }
      });

      // RO often arrives as "97XX-…" or "00…" — strip the model prefix for display
      // when it looks like "97" / "00" + digits, otherwise show as-is.
      let reference = fields.RO || '';
      if (/^(97|00)/.test(reference) && reference.length > 2) {
        // Keep model code visible: "97 123456…" reads clearer in bank apps.
        reference = reference.slice(0, 2) + ' ' + reference.slice(2);
      }

      // I: is "RSD4376,94" — a currency code then a comma-decimal amount. This is
      // the figure the bank app charges, so the modal prefers it over the stored
      // price. Strip the currency letters and any thousands dots, comma → point.
      let amount = null;
      if (fields.I) {
        const num = fields.I.replace(/^[A-Za-z]+/, '').replace(/\./g, '').replace(',', '.');
        const parsed = Number(num);
        if (Number.isFinite(parsed) && parsed > 0) {
          amount = parsed;
        }
      }

      return {
        recipient: fields.N || '',
        account: fields.R || '',
        reference,
        code: fields.SF || '',
        purpose: fields.S || '',
        amount,
      };
    },

    /**
     * Marks the modal's receipt as paid, then closes. No-op if already paid.
     */
    async markPaidAndClose() {
      const item = this.qrItem;
      if (!item || this.isPaid(item)) {
        this.closeQR();
        return;
      }

      // togglePaid flips unpaid → paid; keep the modal open until it finishes
      // so the button can show "Čuvanje...".
      if (!item.paid) {
        await this.togglePaid(item);
      }
      this.closeQR();
    },

    /**
     * Copy the raw IPS payload for the open modal receipt — fallback when the
     * user cannot scan their own screen. Uses the payload already loaded for
     * the modal details when available.
     */
    async copyIPS() {
      const item = this.qrItem;
      if (!item) {
        return;
      }

      try {
        let payload = this.qrPayload;
        if (!payload) {
          const res = await fetch(`${this.receiptPath(item)}/qr.txt${this.accountQuery(item)}`);
          if (!res.ok) {
            this.qrMissing = true;
            return;
          }
          payload = await res.text();
          this.qrPayload = payload;
          this.qrIPS = this.parseIPS(payload);
        }
        await navigator.clipboard.writeText(payload);
        this.qrCopied = true;
        if (this.qrCopiedTimer) {
          clearTimeout(this.qrCopiedTimer);
        }
        this.qrCopiedTimer = setTimeout(() => {
          this.qrCopied = false;
          this.qrCopiedTimer = null;
        }, 2000);
      } catch (e) {
        console.error('Copy IPS payload failed', e);
      }
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
          (r.period || '').toLowerCase().includes(q) ||
          (r.label || '').toLowerCase().includes(q) ||
          (r.account || '').toLowerCase().includes(q)
        );
      }

      // Newest first, then grouped by provider, then by account so several
      // family-member accounts of one provider sit next to each other.
      items.sort((a, b) =>
        (b.modified || 0) - (a.modified || 0) ||
        (a.provider || '').localeCompare(b.provider || '') ||
        (a.account || '').localeCompare(b.account || '')
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
        const res = await fetch(`/api/receipts/${item.period}/${item.provider}/paid${this.accountQuery(item)}`, {
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
