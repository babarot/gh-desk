// Pure: draws what gh-desk draws, on a grid of cells, as gomi's site does. No
// DOM, no Preact. Each cell is one column wide whatever the font does, and box
// lines are drawn by the stylesheet, so that they meet. check.mjs compares
// the text with what gh-desk draws from the same pull requests.
const COLS=132,LINES=18;
// As columnDefs in internal/tui/list.go.
const LEAD=4,CW={pr:24,author:14,ci:2,review:7,why:10,size:11,age:4};
const GROUPS={you:'requested from you',team:'requested from a team',other:'other',bots:'bots'};
const BOXES={'─':'h','│':'v','╵':'tv','╭':'rdr','╮':'rdl','╰':'rur','╯':'rul','▎':'rail','┃':'thick','└':'ur'};
function cw(ch){const c=ch.codePointAt(0);return c>=0x1100&&(c<=0x115f||(c>=0x2e80&&c<=0xa4cf)||(c>=0xac00&&c<=0xd7a3)||(c>=0xf900&&c<=0xfaff)||(c>=0xfe30&&c<=0xfe4f)||(c>=0xff00&&c<=0xff60)||(c>=0xffe0&&c<=0xffe6)||(c>=0x1f300&&c<=0x1faff))?2:1}
const sw=s=>{let w=0;for(const ch of s)w+=cw(ch);return w};
const fit=(s,n)=>{s=String(s);if(sw(s)<=n)return s+' '.repeat(n-sw(s));let o='',u=0;for(const ch of s){if(u+cw(ch)>n-1)break;o+=ch;u+=cw(ch)}return o+'…'+' '.repeat(n-1-u)};
const shortAge=h=>h<24?`${h}h`:h<720?`${Math.floor(h/24)}d`:h<8760?`${Math.floor(h/720)}mo`:`${Math.floor(h/8760)}y`;
const groupOf=x=>x.bot?'bots':x.requested.includes('me')?'you':x.requested.length?'team':'other';
function mix(a,b,t){const h=s=>[1,3,5].map(i=>parseInt(s.slice(i,i+2),16)),x=h(a),y=h(b);return'#'+x.map((v,i)=>Math.round(v+(y[i]-v)*t).toString(16).padStart(2,'0')).join('')}
const esc=s=>s.replace(/&/g,'&amp;').replace(/</g,'&lt;');
class Grid{
  constructor(w,h){this.w=w;this.h=h;this.c=Array.from({length:h},()=>Array.from({length:w},()=>({ch:' '})))}
  // put draws a run of [text, fg, bg, bold] from column x of row y, cut at the edge.
  put(x,y,segs){if(y<0||y>=this.h)return x;for(const[t,fg,bg,b]of segs)for(const ch of t){const k=cw(ch);if(x+k>this.w)return x;if(x>=0){this.c[y][x]={ch,fg,bg,b};if(k===2)this.c[y][x+1]={ch:'',fg,bg,b}}x+=k}return x}
  band(y,bg,from=0,to=this.w){for(let x=from;x<to;x++)this.c[y][x].bg=bg}
  clear(x,y,w,h,bg){for(let r=y;r<y+h&&r<this.h;r++)for(let c=x;c<x+w&&c<this.w;c++)this.c[r][c]={ch:' ',bg}}
  text(){const rows=this.c.map(r=>r.map(c=>c.ch).join('').replace(/\s+$/,''));while(rows.length&&rows[rows.length-1]==='')rows.pop();return rows}
  // html gives every letter a cell of its own, so that the columns hold
  // whatever the font does.
  html(){return this.c.map(row=>{let out='';for(const c of row){if(c.ch==='')continue;const s=(c.fg?`color:${c.fg};`:'')+(c.bg?`background:${c.bg};`:'')+(c.b?'font-weight:700;':''),a=s?` style="${s}"`:'',shape=BOXES[c.ch];out+=shape?`<span class="c bx ${shape}"${a}></span>`:`<span class="c${cw(c.ch)===2?' w':''}"${a}>${c.ch===' '?' ':esc(c.ch)}</span>`}return`<div class="tr">${out}</div>`}).join('')}
}
// box draws a popup, in the focus color, over what is there.
function box(g,p,x,y,w,title,lines,right){const f=p.blue,h=lines.length+2,t=` ${title} `,r=right?` ${right} `:'';g.clear(x,y,w,h,p.base);
  g.put(x,y,[['╭',f,p.base],[t,p.subtext1,p.base,1],['─'.repeat(Math.max(0,w-2-sw(t)-sw(r))),f,p.base],[r,p.overlay,p.base],['╮',f,p.base]]);
  lines.forEach((l,i)=>{g.put(x,y+1+i,[['│ ',f,p.base]]);g.put(x+2,y+1+i,l.map(s=>[s[0],s[1],s[2]??p.base,s[3]]));g.put(x+w-2,y+1+i,[[' │',f,p.base]])});
  g.put(x,y+h-1,[['╰'+'─'.repeat(w-2)+'╯',f,p.base]]);return g}
const MENU={ask_claude:['ask claude about it','talk with claude about its diff'],browse:['browser','open it on github.com'],review:['review','comment, approve or request changes in a review'],comment:['comment','comment on its conversation'],approve:['approve','send an approving review'],merge:['merge','merge it, as the next popup decides'],ready_for_review:['ready for review','not a draft'],close:['close','close it, as the next popup says'],hide:['hide / done','hide it from this tab']};
const status=(g,y,p,parts)=>(g.clear(0,y,g.w,1),g.put(0,y,parts.flatMap(([k,w],i)=>[...(i?[[' · ',p.dim]]:[]),[k,p.blue,null,1],[' '+w,p.dim]])));
const SCENES={
  menu(g,o){const p=o.p,x=o.cur,b=[[[x.title,p.text]],[[' ']]];o.menu.forEach((a,i)=>{const m=MENU[a];b.push([[i===0?'▎ ':'  ',p.blue],[fit(m[0],22),i===0?p.text:p.subtext1],[fit(o.key(a)||'',7),p.mauve,null,1],[m[1],p.overlay]])});b.push([[' ']],[['enter run · or the action\'s key · esc close',p.dim]]);box(g,p,24,1,84,`${x.repo}#${x.n}`,b);status(g,g.h-1,p,[['enter','choose'],['esc','back']])},
  merge(g,o){const p=o.p,x=o.cur,b=[[['✓ ',p.green],[`${x.repo}#${x.n} `,p.subtext1],[x.title,p.text]],[['  └ approve → merge now',p.overlay]],[[' ']],[['GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read.',p.yellow]],[[' ']],[['approve and merge this pull request? ',p.text],['y',p.blue,null,1],[' yes · ',p.dim],['esc',p.blue,null,1],[' no',p.dim]]];box(g,p,3,2,126,'approve and merge',b)},
  compose(g,o){const p=o.p,x=o.cur,b=[[[x.title,p.text]],[[' comment ',p.text,p.surface1,1],['  approve   request changes ',p.overlay]],[[' ']],[['┃ ',p.blue],['Write the review (markdown)',p.overlay]],...Array.from({length:9},()=>[['┃',p.blue]]),[[' ']],[[`ctrl+s post · tab kind · ${o.key('compose_editor')||'ctrl+e'} ${o.editor||'$EDITOR'} · esc discard`,p.dim]]];box(g,p,16,1,100,`${x.repo}#${x.n} · review`,b)},
  keys(g,o){const p=o.p,rows=[['next_tab','next tab'],['prev_tab','previous tab'],['menu','actions'],['select','select'],['select_shown','select shown'],['select_rule','select by rule'],['approve','approve'],['merge','merge'],['review','review'],['ask_claude','ask claude about it'],['hide','hide / done']],b=[[['▾ ',p.subtext0],['the list and its preview ',p.subtext1,null,1],['· works here ',p.overlay],['─'.repeat(20),p.surface1]]];for(const[a,w]of rows)b.push([['  '],[fit(o.keysOf(a).join(' ')||'',16),o.custom(a)?p.mauve:p.blue,null,1],[w,p.subtext0]]);box(g,p,34,1,64,'keys · z fold · ? esc close',b,'↓ j k wheel')},
  rules(g,o){const p=o.p,hit=r=>o.shown.filter(x=>matchRule(r,x)),rs=[{name:'approved by me',note:'approved by you · bots',m:o.shown.filter(x=>x.bot&&x.approved_by_me)},...o.rules.map(r=>({name:r.name||'',note:o.say(r),m:hit(r)}))],c=Math.min(o.ruleAt+1,rs.length-1),b=rs.slice(0,6).map((r,i)=>[[i===c?'▎   ':'    ',p.blue],[fit(r.name,20),i===c?p.text:p.subtext1],[fit(`${r.m.length} PRs`,9),p.overlay],[r.note,p.overlay]]);b.push([[' ']],[['matches',p.subtext1,null,1]]);const m=rs[c].m;b.push(...(m.length?m.slice(0,3).map(x=>[[`  ${x.repo}#${x.n} `,p.subtext1],[x.title,p.text]]):[[['  nothing in this tab',p.overlay]]]),[[' ']],[['space check · enter select these · a add them · f show only these · esc cancel',p.dim]]);box(g,p,11,1,110,'select by rule',b);status(g,g.h-1,p,[['space','check a rule'],['enter','choose'],['a','add to the selection'],['f','show only these'],['esc','back']])},
  pane(g,o){const p=o.p,x=o.cur,f=p.surface1,w=COLS,h=8,y=g.h-1-h,d=o.pane.tab==='diff',t=[' overview ',' diff (2) '],l=w-37;g.clear(0,y,w,h,p.base);const tabs=[[t[0],d?p.overlay:p.text,d?p.base:p.surface0,d?0:1],[' '],[t[1],d?p.text:p.overlay,d?p.surface0:p.base,d?1:0]],tw=tabs.reduce((n,s)=>n+s[0].length,0),right=` ${x.repo}#${x.n} · J/K · v `;
    g.put(0,y,[['╭ ',f,p.base],...tabs.map(s=>[s[0],s[1],s[2]??p.base,s[3]]),[' '],['─'.repeat(w-3-tw-sw(right)-1),f,p.base],[right,p.overlay,p.base],['╮',f,p.base]]);
    for(let i=1;i<h-1;i++){g.put(0,y+i,[['│',f,p.base]]);g.put(w-1,y+i,[['│',f,p.base]])}g.put(0,y+h-1,[['╰'+'─'.repeat(w-2)+'╯',f,p.base]]);
    if(d){[['client.go',p.mauve,1],['@@ -1,3 +1,4 @@ package client',p.teal],['-golang.org/x/net v0.33.0',p.red],['+golang.org/x/net v0.34.1',p.green]].forEach(([s,c,b],i)=>g.put(2,y+1+i,[[s,c,p.base,b]]))}
    else{const md=o.pane.style==='plain';g.put(2,y+1,[[`#${x.n} `,p.overlay,p.base],[x.title,p.text,p.base,1]]);g.put(l-sw(x.author)-3,y+1,[[x.author,p.subtext0,p.base]]);g.put(2,y+2,[['▾ ',p.subtext0,p.base],['description ',p.subtext1,p.base,1],['─'.repeat(l-17),f,p.base]]);g.put(2,y+3,[['│ ',f,p.base],[md?'## What':'What',md?p.text:p.mauve,p.base,md?0:1]]);g.put(2,y+4,[['│ ',f,p.base],['Update the dependency to the release with the fix.',p.text,p.base]]);g.put(2,y+5,[['╵',f,p.base]]);
      for(let i=1;i<h-1;i++)g.put(l-1,y+i,[['│',f,p.base]]);[['status','open',p.green],['requested','me',p.subtext1],['checks','●',p.yellow],['size',`+${x.add} −${x.del} · ${x.files} files`,p.subtext1],['updated',`${shortAge(x.updatedHours)} ago`,p.subtext1]].forEach(([a,b,c],i)=>g.put(l+1,y+1+i,[[fit(a,10),p.overlay,p.base],[b,c,p.base]]))}},
  claude(g,o){const p=o.p,c=o.claude,x=o.cur,f=p.blue,w=COLS,h=10,y=g.h-1-h,a=c.lang==='Japanese'?'⏺ このPRは設定画面にダークテーマを追加します。':`⏺ This adds a dark theme to the settings page. (${c.lang})`,r=` ${c.model} · at ${String(x.n).padEnd(7,'a')} · read-only · pgup/pgdown `,t=` claude on ${x.repo}#${x.n} `;g.clear(0,y,w,h,p.base);
    g.put(0,y,[['╭',f,p.base],[t,p.subtext1,p.base,1],['─'.repeat(w-2-sw(t)-sw(r)),f,p.base],[r,p.overlay,p.base],['╮',f,p.base]]);for(let i=1;i<h-1;i++){g.put(0,y+i,[['│',f,p.base]]);g.put(w-1,y+i,[['│',f,p.base]])}g.put(0,y+h-1,[['╰'+'─'.repeat(w-2)+'╯',f,p.base]]);
    g.put(4,y+2,[[a,p.text,p.base]]);g.put(6,y+3,[['1 tool · $0.40',p.dim,p.base]]);box(g,p,2,y+4,100,'ask claude',[[['┃ ',p.blue],['Ask Claude',p.overlay]],[['┃',p.blue]]])},
};
// o: p palette, cols, tabs, exclude, owners, stale, mark rule, hide rule, spin, keys for the status line, scene, all
function drawTui(o){const p=o.p,hidden=x=>o.exclude.some(r=>matchRule(r,x)),shown=PRS.filter(x=>!hidden(x)),listed=o.hide?PRS:shown,tab=o.tabs[0]||{},grouped=!!(tab.groups&&tab.groups.length),g=new Grid(COLS,o.all?40:LINES);
  const cols=o.cols.filter(c=>c!=='why'||grouped),W={...CW,title:Math.max(10,COLS-1-LEAD-cols.filter(c=>c!=='title').reduce((n,c)=>n+CW[c]+2,0)-2)},tabBg=mix(p.base,p.blue,p.dark?.35:.25),pick=mix(p.base,p.mauve,p.dark?.2:.14);
  let x=g.put(0,0,[['desk',p.blue,p.surface0,1],[' // ',p.dim],['@octocat',p.overlay],['   ']]);
  o.tabs.forEach((t,i)=>{const n=t.kind==='inbox'?3:i===0?shown.length:i+1;x=g.put(x,0,i===0?[[` ${i+1} `,p.subtext0,tabBg],[`${t.name||'inbox'} (${n}) `,p.text,tabBg,1],['  ']]:[[`${i+1} `,p.dim],[t.name||'inbox',p.subtext0],[` (${n})`,p.dim],['   ']])});
  const c={you:0,team:0,other:0,bots:0};shown.forEach(z=>c[groupOf(z)]++);const hasNote=o.mark||o.hide,note=[...(o.mark?[`${shown.filter(z=>matchRule(o.mark,z)).length} selected`]:o.hide?[`${PRS.filter(z=>matchRule(o.hide,z)).length} excluded`]:[]),...(hasNote?[]:[c.you&&`${c.you} to you`,c.team&&`${c.team} to a team`,c.other&&`${c.other} other`,c.bots&&`${c.bots} bots`].filter(Boolean))].join(' · ')+(o.spin?` ${o.spin}`:'');
  g.put(COLS-sw(note),0,[[note,o.mark?p.mauve:p.subtext0]]);
  const head=q=>({ci:'CI'}[q]||q.toUpperCase());let cx=LEAD;for(const q of cols){g.put(cx,1,[[q==='age'?head(q).padStart(W[q]):fit(head(q),W[q]),p.subtext1,null,1]]);cx+=W[q]+2}
  const cell=(q,z,dim)=>{const d=v=>dim?p.dim:v;switch(q){
    case'pr':{const own=z.repo.split('/')[0],r=o.owners.includes(own)?z.repo.split('/')[1]:z.repo,num='#'+z.n,n=z.draft||dim;return[[r,n?p.dim:p.subtext1],[num,n?p.dim:p.overlay]]}
    case'title':return[[fit(z.title,W.title),z.draft?p.dim:d(p.text)]];case'author':return[[fit(z.author,W.author),d(p.subtext1)]];
    case'ci':{const m={passing:['✓',p.green],failing:['✗',p.red],pending:['●',p.yellow]}[z.checks];return[[m[0],d(m[1])]]}
    case'review':{const m={approved:['✓',p.green],changes_requested:['±',p.red],review_required:['○',p.overlay],none:['·',p.dim]}[z.decision];return[[m[0],d(m[1])]]}
    case'why':return[[fit(z.why,W.why),d(p.peach)]];
    case'size':{const a='+'+z.add,b='−'+z.del;return[[a,d(p.green)],[' '],[b,d(p.red)]]}
    case'age':return[[shortAge(z.updatedHours).padStart(W.age),o.stale!=='off'&&z.updated>=o.stale?p.yellow:d(p.dim)]]}};
  let y=2,first=true,cur=null;const room=g.h-1;
  const row=z=>{if(y>=room)return;const m=o.mark&&matchRule(o.mark,z),h=o.hide&&(matchRule(o.hide,z)||hidden(z)),on=first;if(on)cur=z;first=false;const mk=m?['✓',p.mauve]:h?['⊘',p.dim]:null;g.put(0,y,[[on?'▎':' ',p.blue],[' '],[mk?mk[0]:' ',mk?mk[1]:null]]);let cx=LEAD;for(const q of cols){g.put(cx,y,cell(q,z,h));cx+=W[q]+2}if(on||m)g.band(y,m?pick:p.surface0);y++};
  const newest=(a,b)=>a.updatedHours-b.updatedHours;
  if(grouped)for(const k of tab.groups){const rs=listed.filter(z=>groupOf(z)===k).sort(newest);if(!rs.length||y>=room)continue;g.put(0,y++,[['  ▾ ',p.subtext0],[GROUPS[k],p.subtext0,null,1],[' '],[String(rs.length),p.dim]]);rs.forEach(row)}
  else listed.slice().sort(newest).forEach(row);
  if(o.all)return g;
  const k=o.key||(()=>'');status(g,g.h-1,p,[[k('menu')||'enter','actions'],[k('select')||'space','select'],[k('select_rule')||'S','select by rule'],[k('triage')||'C','triage with claude'],[k('log')||'L','log'],[k('filter')||'/','filter'],[k('help')||'?','keys']]);
  const s=o.scene&&SCENES[o.scene];if(s)s(g,{...o,cur:cur||PRS[0],shown});return g}
