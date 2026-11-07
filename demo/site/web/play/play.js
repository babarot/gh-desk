// The playground: the terminal that runs gh-desk.wasm and the key buttons
// under it.
//
// ?scenario=<name> plays a scenario of demo/internal/playground once
// gh-desk has read its lists: its steps are typed one by one, each shown in
// a caption, until the visitor types, with a player under the terminal
// that shows the steps, pauses, goes to a step and plays again. ?watch=1
// only shows: the visitor's keys and clicks do not reach gh-desk. ?inline=1
// lets a frame large enough to drive in keep the visitor when they stop
// watching. ?theme=dark|light sets the theme. The script in the head reads watch and
// theme, and sees whether the page is in a frame.
(() => {
  const $ = id => document.getElementById(id);
  const root = document.documentElement;
  const screen = $('screen');
  const query = new URLSearchParams(location.search);
  const scenario = query.get('scenario') || '';

  // In a frame, the link opens the screen on the whole window, where the
  // visitor drives; watching on the whole window or inline, it lets the
  // visitor drive right there (drive, below).
  const embedded = root.classList.contains('embed');
  const inline = query.has('inline') && query.get('inline') !== '0';
  if (embedded || root.classList.contains('watch')) {
    const full = new URLSearchParams(query);
    full.delete('watch');
    full.delete('inline');
    const s = full.toString();
    $('full').href = s ? '?' + s : './';
    if (root.classList.contains('watch')) $('full').textContent = 'Try it yourself ↗';
  }

  // The caption shows a step for a while after it is typed.
  const caption = $('caption');
  let captionTimer;
  function step(key, help) {
    caption.replaceChildren();
    const k = document.createElement('kbd');
    k.textContent = key;
    caption.append(k);
    if (help) {
      const h = document.createElement('span');
      h.className = 'help';
      h.textContent = help;
      caption.append(h);
    }
    caption.hidden = false;
    clearTimeout(captionTimer);
    captionTimer = setTimeout(() => { caption.hidden = true; }, 1800);
  }

  // The player follows a scenario as a video's controls do. The steps are
  // placed on the track by when each is typed; the fill moves between two
  // steps by the time the next one waits. Going to a step plays the
  // scenario again from the start, fast up to that step: a screen of
  // gh-desk cannot be wound back.
  // watch is let: on the whole window the visitor can stop watching and
  // drive the screen being shown.
  let watch = root.classList.contains('watch');
  // END is a step past the last: playing from it types every step fast.
  const END = 1 << 20;
  // A seek plays the scenario again from the start in a new gh-desk of the
  // same module, with the world hurried and the steps before the one gone
  // to typed fast. What it writes meanwhile is held, not drawn, and drawn
  // at once when it gets there: the screen goes from where it was to the
  // step, as a video does. seekTo is that step; held is what was written,
  // or null when no seek is on its way.
  const pl = { steps: [], at: [], total: 1, cur: -1, since: 0, pausedAt: 0, state: 'off', seekTo: 0, held: null };
  const loading = $('loading');
  function cover(text) {
    loading.textContent = text;
    // The player stays uncovered, so the visitor sees where it goes.
    loading.style.bottom = playerEl.hidden ? '' : playerEl.offsetHeight + 'px';
    loading.hidden = false;
  }
  function uncover() { loading.hidden = true; }
  const playerEl = $('player'), fill = $('fill'), track = $('track'), now = $('now'), pp = $('pp');
  // Watching a scenario, the player is there from the start, so it reads
  // as a video before anything plays; its steps come with gh-desk.
  if (watch && scenario) {
    playerEl.hidden = false;
    now.textContent = 'loading…';
    cover('Loading gh-desk…');
  }

  function showNow() {
    now.replaceChildren();
    if (pl.state === 'taken') { now.textContent = 'you are driving'; return; }
    if (pl.state === 'done') { now.textContent = 'done'; return; }
    if (pl.cur < 0) { now.textContent = `0/${pl.steps.length}`; return; }
    // On the way to the end of a scenario (END), no step is shown.
    const st = pl.steps[pl.cur];
    if (!st) return;
    const k = document.createElement('kbd');
    k.textContent = st.key;
    now.append(`${pl.cur + 1}/${pl.steps.length} `, k, st.help ? ' ' + st.help : '');
  }
  function marks() {
    track.querySelectorAll('.mark').forEach((m, i) => m.classList.toggle('past', i <= pl.cur));
  }
  function onReady(steps) {
    pl.steps = steps;
    let t = 0;
    pl.at = steps.map(st => (t += st.delay));
    pl.total = t || 1;
    // A replay of the same scenario keeps the marks there are.
    const old = track.querySelectorAll('.mark');
    if (old.length !== steps.length) old.forEach(m => m.remove());
    if (old.length !== steps.length) steps.forEach((st, i) => {
      const m = document.createElement('i');
      m.className = 'mark';
      m.style.left = (pl.at[i] / pl.total * 100) + '%';
      m.title = `${i + 1}. ${st.key}${st.help ? ' · ' + st.help : ''}`;
      track.append(m);
    });
    pl.cur = -1; pl.since = performance.now(); pl.state = 'playing'; pl.pausedAt = 0;
    pp.textContent = '❚❚'; pp.setAttribute('aria-label', 'Pause');
    playerEl.hidden = !watch;
    if (pl.held && pl.seekTo === 0) release();
    // On the way to a step, the player keeps showing that step.
    if (pl.held) { pl.cur = pl.seekTo - 1; marks(); return; }
    showNow(); marks();
  }
  function onStep(i) {
    pl.cur = i; pl.since = performance.now();
    if (pl.pausedAt) pl.pausedAt = pl.since;
    const st = pl.steps[i];
    if (pl.held) {
      // On the way: nothing is drawn until the step before the one gone to.
      if (i < pl.seekTo - 1) { pl.cur = pl.seekTo - 1; return; }
      release();
      showNow(); marks();
      return;
    }
    step(st.key, st.help);
    showNow(); marks();
  }
  // No scenario has the name: gh-desk starts as it does without one, so the
  // player has nothing to show and the screen nothing to wait for.
  function onUnknown() {
    pl.state = 'off';
    playerEl.hidden = true;
    if (pl.held) { release(); uncover(); }
  }
  function onDone() {
    if (pl.held) { release(); uncover(); }
    pl.state = 'done';
    pp.textContent = '↺'; pp.setAttribute('aria-label', 'Play from the start');
    showNow();
  }
  function taken() {
    if (pl.state !== 'playing') return;
    pl.state = 'taken';
    pp.textContent = '↺'; pp.setAttribute('aria-label', 'Play from the start');
    showNow();
  }
  (function frame() {
    if (pl.held) {
      fill.style.width = (pl.seekTo ? pl.at[pl.seekTo - 1] / pl.total * 100 : 0) + '%';
    } else if (pl.state === 'playing') {
      const t = pl.pausedAt || performance.now();
      const base = pl.cur >= 0 ? pl.at[pl.cur] : 0;
      const next = pl.steps[pl.cur + 1];
      const pos = next ? base + Math.min(t - pl.since, next.delay) : pl.total;
      fill.style.width = (pos / pl.total * 100) + '%';
    } else if (pl.state === 'done') {
      fill.style.width = '100%';
    }
    requestAnimationFrame(frame);
  })();

  // write draws what gh-desk writes, or holds it while a seek is on its
  // way. Held, it is not seen by the terminal, which would answer gh-desk's
  // question for its background; the page answers it instead, as the
  // terminal would.
  function write(s) {
    if (!pl.held) { term.write(s); return; }
    pl.held.push(s);
    if (s.includes('\x1b]11;?') && globalThis.ghDesk) {
      const hex = themes[dark() ? 'dark' : 'light'].background.slice(1);
      const c = i => hex.slice(i, i + 2).repeat(2);
      globalThis.ghDesk.input(`\x1b]11;rgb:${c(0)}/${c(2)}/${c(4)}\x1b\\`);
    }
  }
  function release() {
    const s = pl.held.join('');
    pl.held = null;
    term.write(s);
  }
  function replay(from) {
    const api = globalThis.ghDesk;
    if (!api || !api.seek || pl.state === 'off') { cover('Loading gh-desk…'); start(); return; }
    if (!watch) from = END;
    pl.seekTo = from;
    pl.held = [];
    pl.pausedAt = 0;
    pl.cur = from - 1;
    pl.state = 'playing';
    pp.textContent = '❚❚'; pp.setAttribute('aria-label', 'Pause');
    showNow(); marks();
    api.pause(false);
    api.seek(from);
    // Should the step never come (an error), the screen is not left still.
    const held = pl.held;
    setTimeout(() => { if (pl.held === held) release(); }, 5000);
  }
  pp.addEventListener('click', () => {
    if (pl.state !== 'playing') { replay(0); return; }
    const api = globalThis.ghDesk;
    if (pl.pausedAt) {
      pl.since += performance.now() - pl.pausedAt;
      pl.pausedAt = 0;
      pp.textContent = '❚❚'; pp.setAttribute('aria-label', 'Pause');
      if (api) api.pause(false);
    } else {
      pl.pausedAt = performance.now();
      pp.textContent = '▶'; pp.setAttribute('aria-label', 'Play');
      if (api) api.pause(true);
    }
  });
  $('again').addEventListener('click', () => replay(0));
  // A click on the track goes to the step nearest it, and plays from that
  // step: the steps before it are typed fast, and it is typed at its pace,
  // so the visitor sees it happen.
  track.addEventListener('click', e => {
    if (!pl.steps.length) return;
    const r = track.getBoundingClientRect();
    const t = (e.clientX - r.left) / r.width * pl.total;
    let near = 0;
    pl.at.forEach((a, i) => { if (Math.abs(a - t) < Math.abs(pl.at[near] - t)) near = i; });
    replay(near);
  });

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
    // Watching, no key reaches gh-desk; the terminal's own answers to it
    // still do, through onData.
    term.attachCustomKeyEventHandler(() => !watch);
    // The WebGL renderer draws the box-drawing characters itself, filling
    // the cell, so the borders join across the line height. The DOM
    // renderer, left when WebGL is missing, draws them from the font.
    try {
      const gl = new WebglAddon.WebglAddon();
      gl.onContextLoss(() => gl.dispose());
      term.loadAddon(gl);
    } catch (e) {}
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
  async function start(from = 0) {
    const me = ++session;
    pl.held = null;
    pl.state = 'off';
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
    // Driving a scenario, gh-desk opens on its screen: the steps are typed
    // fast behind the cover, and the screen is drawn when they are done.
    const jump = scenario && !watch;
    if (jump) { pl.held = []; pl.seekTo = END; from = END; } else uncover();
    const mine = f => (...a) => { if (me === session) f(...a); };
    api.start(term.cols, term.rows, mine(write), {
      scenario, from, hurry: jump,
      onReady: mine(onReady), onStep: mine(onStep), onDone: mine(onDone), onUnknown: mine(onUnknown),
    });
    term.onData(d => api.input(d));
    // A key or a click is the visitor's, and stops a scenario; onData
    // also carries the terminal's answers to gh-desk.
    term.onKey(() => { api.takeOver(); taken(); });
    term.onBinary(d => api.input(d));
    term.onResize(({ cols, rows }) => api.resize(cols, rows));
    // In a frame the page around it keeps the focus until a click.
    if (!watch && !embedded) term.focus();
  }

  new ResizeObserver(size).observe(screen);
  screen.addEventListener('mousedown', () => { if (globalThis.ghDesk) { globalThis.ghDesk.takeOver(); taken(); } });

  const keyOf = { '\\t': '\t', '\\r': '\r', '\\x1b': '\x1b', '\\x18': '\x18' };
  document.querySelectorAll('#keys button').forEach(b => b.addEventListener('click', () => {
    const k = keyOf[b.dataset.k] ?? b.dataset.k;
    if (globalThis.ghDesk) {
      globalThis.ghDesk.takeOver();
      taken();
      globalThis.ghDesk.input(k);
    }
    term.focus();
  }));
  // Inline, a scenario was only shown to the visitor, who starts over at the
  // list; elsewhere the address names the scenario, and it plays again.
  $('restart').addEventListener('click', () => {
    if (inline) { location.replace('./'); return; }
    replay(0);
    if (!watch) term.focus();
  });

  // What is drawn follows the OS, and a switch on the page around this
  // frame or in another tab. A change restarts gh-desk, which reads the
  // terminal's background as it starts.
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (!root.dataset.theme) start();
  });
  addEventListener('storage', e => {
    if (e.key !== 'color-theme') return;
    const was = dark();
    if (e.newValue === 'dark' || e.newValue === 'light') root.dataset.theme = e.newValue;
    else delete root.dataset.theme;
    if (dark() !== was) start();
  });

  document.fonts.load('13px "JetBrains Mono"').finally(start);

  // drive stops watching: the scenario stops where it is, the player goes,
  // and the keys and clicks reach gh-desk from the screen shown.
  function drive() {
    watch = false;
    root.classList.remove('watch');
    if (globalThis.ghDesk) globalThis.ghDesk.takeOver();
    pl.state = 'taken';
    if (pl.held) { release(); uncover(); }
    playerEl.hidden = true;
    $('full').textContent = 'Open the playground ↗';
    const q = new URLSearchParams(location.search);
    q.delete('watch');
    q.delete('inline');
    const rest = q.toString();
    history.replaceState(null, '', location.pathname + (rest ? '?' + rest : '') + location.hash);
    term.focus();
  }
  // In a frame the link leaves it, unless the frame is inline; on the whole
  // window the visitor drives right there.
  if (!embedded || inline) {
    $('full').addEventListener('click', e => {
      if (!watch) return;
      e.preventDefault();
      drive();
    });
  }
})();
