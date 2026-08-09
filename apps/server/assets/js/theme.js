/**
 * Theme toggle.
 *
 * Applying the stored theme is handled by an inline script in the document
 * head (see templates/partials.html) so it runs before first paint. This file
 * only exposes the toggle itself, and is defined immediately rather than on
 * DOMContentLoaded so the header button can never fire before it exists.
 */
window.toggleTheme = function () {
  const isDark = document.documentElement.classList.toggle('dark');

  try {
    localStorage.setItem('theme', isDark ? 'dark' : 'light');
  } catch (_) {
    // Private mode / storage disabled: the toggle still works for this page.
  }

  // Lets components that paint their own colours (the charts) restyle.
  window.dispatchEvent(new CustomEvent('theme-changed', { detail: { dark: isDark } }));
};
