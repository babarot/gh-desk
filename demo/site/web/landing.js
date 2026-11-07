// The landing page: the copy button, the theme, and the cards that play a
// scenario in the frame of the playground. The playground hears of a switch
// of the theme through the key it is kept under.
(() => {
  const $ = id => document.getElementById(id);
  const root = document.documentElement;

  $('copy').addEventListener('click', e => {
    navigator.clipboard.writeText($('cmd').textContent).then(() => {
      e.target.textContent = 'Copied';
      setTimeout(() => { e.target.textContent = 'Copy'; }, 2000);
    });
  });

  // Each click goes on to the next of system, light and dark, as in the docs,
  // whose key keeps it.
  const theme = $('theme');
  const show = () => { theme.dataset.mode = root.dataset.theme || 'system'; };
  theme.addEventListener('click', () => {
    const next = { system: 'light', light: 'dark', dark: 'system' }[root.dataset.theme || 'system'];
    if (next === 'system') delete root.dataset.theme;
    else root.dataset.theme = next;
    try { localStorage.setItem('color-theme', next); } catch (e) {}
    show();
  });
  addEventListener('storage', e => {
    if (e.key !== 'color-theme') return;
    if (e.newValue === 'dark' || e.newValue === 'light') root.dataset.theme = e.newValue;
    else delete root.dataset.theme;
    show();
  });
  show();

  document.querySelectorAll('[data-scenario]').forEach(a => a.addEventListener('click', e => {
    e.preventDefault();
    $('playground').src = `play/?watch=1&inline=1&scenario=${a.dataset.scenario}`;
  }));
})();
