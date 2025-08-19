// @ts-check

document.addEventListener('alpine:init', () => {
  Alpine.store('app', {
    availableThemes: [
      {
        name: 'cyan',
        label: 'Drina',
        color: 'bg-cyan-500',
      },
      {
        name: 'emerald',
        label: 'Zelena Oaza',
        color: 'bg-emerald-500'
      },
      {
        name: 'amber',
        label: 'Jesen',
        color: 'bg-amber-500',
      },
      {
        name: 'pink',
        label: 'Parada',
        color: 'bg-pink-500',
      },
    ],
    theme: '',
    notification: '',
    notificationType: '',
    loading: false,
    receipts: [], // should not be here, but... I am lazy
    setLoading() {
      this.loading = true
    },
    stopLoading() {
      this.loading = false;
    },
    init() {
      const theme = localStorage.getItem('theme');

      if (theme === null) {
        console.warn('No theme');
        // Default theme
        document.documentElement.setAttribute('data-theme', 'cyan');
        return;
      }

      const preference = detectPreference();

      if (preference === 'dark') {
        console.info('prefers: dark')
      } else {
        console.info('prefers: light')
      }

      document.documentElement.setAttribute('data-theme', theme);
      changeFavicon(theme);
      this.theme = theme;
    },
    changeTheme(theme) {
      localStorage.setItem('theme', theme);
      document.documentElement.setAttribute('data-theme', theme);
      this.theme = theme;

      changeFavicon(theme)
    },
    setNotification(message, type, timeout) {
      this.notification = message;
      this.notificationType = type;

      setTimeout(() => {
        this.notification = null;
        this.notificationType = null;
      }, timeout)
    }
  });

  Alpine.data('receipts', () => ({
    filters: [],
    loading: false,
    load() {
      this.loading = true;
      console.log(Alpine.store('app'));

      setTimeout(() => {
        this.loading = false;
      }, 1500)
    }
  }));
});

/**
 *
 * @param {ArrayBuffer} buffer
 * @returns {string}
 */
function arrayBufferToBase64(buffer) {
  let binary = '';
  const bytes = new Uint8Array(buffer);
  const len = bytes.byteLength;

  for (let i = 0; i < len; i++) {
    binary += String.fromCharCode(bytes[i]);
  }

  return window.btoa(binary);
}

/**
 * Detects user's preference
 * @returns {string} Dark (🌙) or Light Mode (☀️️)
 */
function detectPreference() {
  // Check for the user's ️preference
  const darkModeMediaQuery = window.matchMedia('(prefers-color-scheme: dark)').matches;

  return darkModeMediaQuery ? 'dark' : 'light';
}

/**
 * Changes Favicon
 *
 * @param {string} theme
 *
 * @return {void}
 */
async function changeFavicon(theme) {
  const response = await fetch(`/assets/images/${theme}/favicon.ico`)
  const data = await response.arrayBuffer()

  const base64Image = arrayBufferToBase64(data);

  const imgSrc = `data:image/png;base64,${base64Image}`

  const faviconLink = document.querySelector('link[rel="shortcut icon"]');
  faviconLink.setAttribute('href', imgSrc);
}
