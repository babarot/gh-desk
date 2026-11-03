// The page: the terminal that runs gh-desk.wasm, the key buttons under it,
// the copy button and the theme.
(() => {
  const $ = id => document.getElementById(id);
  const root = document.documentElement;
  const screen = $('screen');

  // The terminal's colors follow the page's theme; gh-desk asks the
  // terminal for its background and picks its own theme from it.
  const themes = {
    dark: { background: '#0f1218', foreground: '#d6dae3', cursor: '#d6dae3', selectionBackground: '#33415c' },
    light: { background: '#fbfaf7', foreground: '#2b2f38', cursor: '#2b2f38', selectionBackground: '#cdd7ea' },
  };
  const dark = () => (root.dataset.theme ? root.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches);

  let term, fit, session = 0, module;

  function newTerminal() {
    if (term) term.dispose();
    term = new Terminal({
      fontFamily: '"JetBrains Mono", ui-monospace, Menlo, monospace',
      fontSize: innerWidth < 700 ? 11 : 13,
      lineHeight: 1.15,
      theme: themes[dark() ? 'dark' : 'light'],
      allowProposedApi: true,
      cursorBlink: false,
      // Bubble Tea writes to an output that is not a tty as if the tty
      // turned \n into \r\n, as a cooked one does.
      convertEol: true,
      scrollback: 0,
    });
    fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    // The widths of Unicode 11, as gh-desk counts them: an emoji takes
    // two cells.
    term.loadAddon(new Unicode11Addon.Unicode11Addon());
    term.unicode.activeVersion = '11';
    term.open(screen);
    size();
  }

  // size fits the terminal to the screen, but never narrower than minCols:
  // on a phone the list would cut every title short, so the terminal is
  // made wider than the screen and scrolls sideways instead.
  const minCols = 80;
  function size() {
    const d = fit && fit.proposeDimensions();
    if (!d || !d.cols || !d.rows) return;
    const cols = Math.max(d.cols, minCols);
    if (cols !== term.cols || d.rows !== term.rows) term.resize(cols, d.rows);
  }

  // start runs a new gh-desk in a new terminal. A Go program runs once,
  // so a restart instantiates the module again; the old one is left
  // waiting, writing nowhere.
  async function start() {
    const me = ++session;
    newTerminal();
    const go = new Go();
    if (!module) {
      const resp = fetch('gh-desk.wasm');
      module = await (WebAssembly.compileStreaming ? WebAssembly.compileStreaming(resp) : resp.then(r => r.arrayBuffer()).then(b => WebAssembly.compile(b)));
    }
    const inst = await WebAssembly.instantiate(module, go.importObject);
    const ready = new Promise(res => addEventListener('gh-desk-ready', res, { once: true }));
    go.run(inst);
    await ready;
    if (me !== session) return;
    const api = globalThis.ghDesk;
    $('loading').hidden = true;
    api.start(term.cols, term.rows, s => { if (me === session) term.write(s); });
    term.onData(d => api.input(d));
    term.onBinary(d => api.input(d));
    term.onResize(({ cols, rows }) => api.resize(cols, rows));
    term.focus();
  }

  new ResizeObserver(size).observe(screen);

  const keyOf = { '\\t': '\t', '\\r': '\r', '\\x1b': '\x1b', '\\x12': '\x12' };
  document.querySelectorAll('#keys button').forEach(b => b.addEventListener('click', () => {
    const k = keyOf[b.dataset.k] ?? b.dataset.k;
    if (globalThis.ghDesk) globalThis.ghDesk.input(k);
    term.focus();
  }));
  $('restart').addEventListener('click', () => { $('loading').hidden = false; start(); });

  $('copy').addEventListener('click', e => {
    navigator.clipboard.writeText($('cmd').textContent).then(() => {
      e.target.textContent = 'Copied';
      setTimeout(() => { e.target.textContent = 'Copy'; }, 2000);
    });
  });

  // The theme follows the OS until the visitor picks one. A change
  // restarts gh-desk, which reads the terminal's background as it starts.
  const themeBtn = $('theme');
  const label = () => { themeBtn.textContent = dark() ? 'light' : 'dark'; };
  themeBtn.addEventListener('click', () => {
    root.dataset.theme = dark() ? 'light' : 'dark';
    try { localStorage.setItem('gh-desk-theme', root.dataset.theme); } catch (e) {}
    label();
    start();
  });
  label();

  document.fonts.load('13px "JetBrains Mono"').finally(start);
})();
