// The landing page: the copy button, the theme, the file the settings
// write, and the cards that play a scenario in the frame of the playground. The playground hears of a switch
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

  // The file the settings would write, moved to the last line that changed,
  // as a list of rules is longer than the box. A line's height is the
  // box's, which landing.css sets.
  const toml = $('toml');
  const empty = '# Change a setting, and what gh-desk\n# would write shows here.';
  toml.textContent = empty;
  let shown = '';
  addEventListener('message', e => {
    if (e.origin !== location.origin || e.source !== $('panel').contentWindow) return;
    const text = e.data.config.replace(/^#:schema.*\n/, '').trim();
    const was = shown.split('\n'), now = text.split('\n');
    let same = 0;
    while (same < was.length && same < now.length && was[was.length - 1 - same] === now[now.length - 1 - same]) same++;
    shown = text;
    toml.textContent = text || empty;
    const line = parseFloat(getComputedStyle(toml).lineHeight);
    toml.scrollTop = (now.length - same) * line - toml.clientHeight / 2;
  });

  document.querySelectorAll('[data-scenario]').forEach(a => a.addEventListener('click', e => {
    e.preventDefault();
    $('playground').src = `play/?watch=1&inline=1&scenario=${a.dataset.scenario}`;
  }));
})();
