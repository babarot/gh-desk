// Checks the page of config ui without a browser (mise run config-ui:test):
// that it draws the list as gh-desk does, that it has a place for every
// setting, and what its code writes. With --update, output.toml is written;
// a Go test then reads it as gh-desk reads a config file.
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const read = p => readFileSync(join(here, p), 'utf8');
const { DATA, KEYS } = JSON.parse(read('testdata/data.json'));
const PRS = JSON.parse(read('web/prs.json'));
const mod = new Function('DATA', 'KEYS', 'PRS', read('web/model.js') + read('web/tui.js') + ';return {load,clone,genToml,problems,drawTui,defaultTabs,COLS,LINES};')(DATA, KEYS, PRS);

let failed = false;
const fail = m => { failed = true; console.error(m); };

// The list, as text; colors are the page's business.
const p = new Proxy({}, { get: () => '#000000' });
const def = k => DATA.settings.find(s => s.key === k).defv;
const tabs = mod.defaultTabs();
const rows = mod.drawTui({ p, all: true, cols: def('list.columns'), tabs, exclude: [], owners: [], stale: def('list.stale_days') }).text().slice(1);
const got = rows.join('\n') + '\n';
const want = read('testdata/list.golden');
if (got !== want) fail(`the list is not what gh-desk draws:\n--- gh-desk\n${want}--- page\n${got}`);

// The cursor stays at the start of the line, wherever the columns are.
const moved = mod.drawTui({ p, all: true, cols: ['title', 'pr', 'author'], tabs, exclude: [], owners: [], stale: def('list.stale_days') }).text();
const atCursor = moved.find(l => l.startsWith('▎'));
if (!atCursor || atCursor.indexOf('acme/web#228') < atCursor.indexOf('Add a dark')) fail(`the cursor moved with its column: ${atCursor}`);

// The popups the same way, frame and text, wherever they are on the screen.
const crop = lines => { const y = lines.findIndex(l => l.includes('╭')), x = lines[y].indexOf('╭'), w = lines[y].indexOf('╮') - x + 1; const out = []; for (const l of lines.slice(y)) { out.push(l.slice(x, x + w)); if (l[x] === '╰') break; } return out.join('\n').replace(/ +(\n|$)/g, '$1'); };
const scenes = Object.fromEntries(read('testdata/scenes.golden').split(/^=== /m).slice(1).map(s => { const i = s.indexOf('\n'); return [s.slice(0, i), s.slice(i + 1).trimEnd()]; }));
const k = a => (KEYS.find(x => x.action === a)?.keys || [])[0];
for (const scene of Object.keys(scenes)) {
  const lines = mod.drawTui({ p, cols: def('list.columns'), tabs, exclude: [], owners: [], stale: def('list.stale_days'), scene, key: k, menu: def('menu.actions') }).text().slice(1);
  const page = crop(lines);
  if (page !== scenes[scene]) fail(`the popup ${scene} is not what gh-desk draws:\n--- gh-desk\n${scenes[scene]}\n--- page\n${page}`);
}

// A place for every setting.
const app = read('web/app.js');
for (const s of DATA.settings) if (app.split(`'${s.key}'`).length < 3) fail(`${s.key} has no label or no page in web/app.js`);
for (const a of DATA.arrays) if (!app.includes(`['${a.name}',`)) fail(`[[${a.name}]] has no page in web/app.js`);
for (const e of Object.values(DATA.elems).flat()) if (e.key !== 'name' && !new RegExp(`\\b${e.key}:\\[`).test(app) && !['kind', 'groups', 'folded', 'sort', 'limit', 'show_settled'].includes(e.key)) fail(`the condition ${e.key} has no words in web/app.js`);

// What it writes, from a file that uses every shape of value. A Go test
// reads it back and compares it with input.json.
const file = JSON.parse(read('testdata/input.json'));
const d = mod.load(file);
if (mod.problems(d).length) fail('problems: ' + JSON.stringify(mod.problems(d)));
const out = mod.genToml(d);
if (process.argv.includes('--update')) writeFileSync(join(here, 'testdata/output.toml'), out);
else if (out !== read('testdata/output.toml')) fail('testdata/output.toml is not what the page writes; run node internal/config/ui/check.mjs --update');

process.exit(failed ? 1 : 0);
