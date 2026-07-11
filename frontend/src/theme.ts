/** Enable Semi dark mode for the whole Blog SPA (public + admin). */
export function applyDarkTheme() {
  document.body.setAttribute('theme-mode', 'dark');
  document.documentElement.setAttribute('theme-mode', 'dark');
  document.documentElement.style.colorScheme = 'dark';
}
