/**
 * Refresh control in the site header.
 *
 * Triggers the same provider downloads as the `checks` CLI command. The server
 * runs them in the background and this polls for completion, because a full run
 * talks to several external providers and can take a while.
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('refreshControl', () => ({
    running: false,
    allowed: true,
    checkUntil: 20,
    startedAt: 0,
    finishedAt: 0,
    results: [],
    timer: null,
    // Re-render the relative timestamp periodically without polling the server.
    tick: 0,

    async init() {
      await this.fetchState();

      if (this.running) {
        this.schedulePoll();
      }

      // Keeps "pre 5 minuta" honest while the page sits open.
      setInterval(() => this.tick++, 30000);
    },

    /**
     * @returns {boolean}
     */
    get canRefresh() {
      return this.allowed && !this.running;
    },

    /**
     * @returns {string}
     */
    get refreshTitle() {
      if (this.running) {
        return 'Preuzimanje u toku...';
      }

      if (!this.allowed) {
        return `Preuzimanje je dostupno samo do ${this.checkUntil}. u mesecu`;
      }

      return 'Preuzmi nove račune';
    },

    /**
     * @returns {string}
     */
    get lastFetchedLabel() {
      // Touch tick so Alpine recomputes this on the interval above.
      this.tick;

      if (this.running) {
        return 'U toku...';
      }

      if (!this.finishedAt) {
        return 'Nikad';
      }

      return this.relative(this.finishedAt);
    },

    /**
     * Tooltip with the per-provider outcome of the last run.
     *
     * @returns {string}
     */
    get lastFetchedTitle() {
      if (!this.finishedAt) {
        return 'Računi još nisu preuzimani';
      }

      const when = new Date(this.finishedAt * 1000).toLocaleString('sr-Latn-RS');
      if (this.results.length === 0) {
        return when;
      }

      const lines = this.results.map(r =>
        `${r.provider.toUpperCase()}: ${r.ok ? 'uspešno' : (r.error || 'greška')}`
      );

      return `${when}\n${lines.join('\n')}`;
    },

    /**
     * @returns {number}
     */
    get failedCount() {
      return this.results.filter(r => !r.ok).length;
    },

    async fetchState() {
      try {
        const res = await fetch('/api/refresh');
        if (!res.ok) {
          throw new Error(`failed to fetch refresh state: ${res.status}`);
        }

        this.apply(await res.json());
      } catch (e) {
        console.error(e);
      }
    },

    async start() {
      if (!this.canRefresh) {
        return;
      }

      // Optimistic, so the spinner reacts on the very first click.
      this.running = true;

      try {
        const res = await fetch('/api/refresh', { method: 'POST' });

        // Outside the check_until window: re-enable the disabled state.
        if (res.status === 403) {
          this.apply(await res.json());
          this.running = false;
          return;
        }

        // 409 means someone already kicked one off; polling still applies.
        if (!res.ok && res.status !== 409) {
          throw new Error(`failed to start refresh: ${res.status}`);
        }

        this.apply(await res.json());
        this.schedulePoll();
      } catch (e) {
        console.error(e);
        this.running = false;
      }
    },

    schedulePoll() {
      clearTimeout(this.timer);
      this.timer = setTimeout(() => this.poll(), 2000);
    },

    async poll() {
      const wasRunning = this.running;

      await this.fetchState();

      if (this.running) {
        this.schedulePoll();
        return;
      }

      if (wasRunning) {
        // New PDFs may have landed, so let the receipt list reload itself.
        window.dispatchEvent(new CustomEvent('receipts-updated'));
      }
    },

    /**
     * @param {object} state
     */
    apply(state) {
      this.running = !!state.running;
      this.allowed = state.allowed !== false;
      this.checkUntil = Number(state.check_until || 20);
      this.startedAt = Number(state.started_at || 0);
      this.finishedAt = Number(state.finished_at || 0);
      this.results = Array.isArray(state.results) ? state.results : [];
    },

    /**
     * Human-readable "time ago" in Serbian.
     *
     * @param {number} unixSeconds
     *
     * @returns {string}
     */
    relative(unixSeconds) {
      const seconds = Math.floor(Date.now() / 1000) - unixSeconds;

      const units = [
        ['second', 60],
        ['minute', 60],
        ['hour', 24],
        ['day', 30],
        ['month', 12],
        ['year', Infinity],
      ];

      let value = seconds;
      let unit = 'second';

      for (const [name, size] of units) {
        unit = name;
        if (Math.abs(value) < size) {
          break;
        }
        value = Math.round(value / size);
      }

      try {
        return new Intl.RelativeTimeFormat('sr-Latn-RS', { numeric: 'auto' })
          .format(-value, unit);
      } catch (_) {
        return new Date(unixSeconds * 1000).toLocaleString('sr-Latn-RS');
      }
    },
  }));
});
