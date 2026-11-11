// Pure: draws what gh-desk draws, on a grid of cells, as gomi's site does. No
// DOM, no Preact. Each cell is one column wide whatever the font does, and box
// lines are drawn by the stylesheet, so that they meet. check.mjs compares
// the text with what gh-desk draws from the same pull requests.
const COLS=132,LINES=18;
// As columnDefs in internal/tui/list.go.
const LEAD=4,CW={pr:24,author:14,ci:2,merge:5,review:7,why:10,size:11,age:4};
const BOXES={'─':'h','│':'v','╵':'tv','╭':'rdr','╮':'rdl','╰':'rur','╯':'rul','▎':'rail','┃':'thick','└':'ur'};
function cw(ch){const c=ch.codePointAt(0);return c>=0x1100&&(c<=0x115f||(c>=0x2e80&&c<=0xa4cf)||(c>=0xac00&&c<=0xd7a3)||(c>=0xf900&&c<=0xfaff)||(c>=0xfe30&&c<=0xfe4f)||(c>=0xff00&&c<=0xff60)||(c>=0xffe0&&c<=0xffe6)||(c>=0x1f300&&c<=0x1faff))?2:1}
const sw=s=>{let w=0;for(const ch of s)w+=cw(ch);return w};
const fit=(s,n)=>{s=String(s);if(sw(s)<=n)return s+' '.repeat(n-sw(s));let o='',u=0;for(const ch of s){if(u+cw(ch)>n-1)break;o+=ch;u+=cw(ch)}return o+'…'+' '.repeat(n-1-u)};
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
const MENU={ask_claude:['ask claude about it','talk with claude about its diff'],browse:['browser','open it on github.com'],review:['review','comment, approve or request changes in a review'],comment:['comment','comment on its conversation'],approve:['approve','send an approving review'],merge:['merge','merge it, as the next popup decides'],draft:['draft / ready',"someone else's pull request"],close:['close','close it, as the next popup says'],hide:['hide / done','hide it from this tab']};
const status=(g,y,p,parts)=>(g.clear(0,y,g.w,1),g.put(0,y,parts.flatMap(([k,w],i)=>[...(i?[[' · ',p.dim]]:[]),[k,p.blue,null,1],[' '+w,p.dim]])));
const SCENES={
  merge(g,o){const p=o.p,x=o.cur,b=[[['✓ ',p.green],[`${x.repo}#${x.n} `,p.subtext1],[x.title,p.text]],[['  └ approve → merge now',p.overlay]],[[' ']],[['GitHub cannot take an approval or a merge back. Each acts only on the head shown when it was read.',p.yellow]],[[' ']],[['approve and merge this pull request? ',p.text],['y',p.blue,null,1],[' yes · ',p.dim],['esc',p.blue,null,1],[' no',p.dim]]];box(g,p,3,2,126,'approve and merge',b)},
  keys(g,o){const p=o.p,rows=[['next_tab','next tab'],['prev_tab','previous tab'],['menu','actions'],['select','select'],['select_shown','select shown'],['select_rule','select by rule'],['approve','approve'],['merge','merge'],['review','review'],['ask_claude','ask claude about it'],['hide','hide / done']],b=[[['▾ ',p.subtext0],['the list and its preview ',p.subtext1,null,1],['· works here ',p.overlay],['─'.repeat(20),p.surface1]]];for(const[a,w]of rows)b.push([['  '],[fit(o.keysOf(a).join(' ')||'',16),o.custom(a)?p.mauve:p.blue,null,1],[w,p.subtext0]]);box(g,p,34,1,64,'keys · z fold · ? esc close',b,'↓ j k wheel')},
};
// o: p palette, cols, tabs, exclude, owners, stale, mark rule, hide rule, spin, keys for the status line, scene (merge or keys, the scenes the settings playground shows), all
function drawTui(o){const p=o.p,prs=o.prs||[],v=o.v||(()=>'n'),at=o.at||0,lay=o.layout&&o.layout.names.length?o.layout:null,placed=x=>lay?lay.at[x.i]:-1,hidden=x=>o.exclude.some((r,i)=>v('exclude',i,x)==='y')||(lay&&lay.show[placed(x)]==='excluded'&&!lay.pending.has(x.i)),shown=prs.filter(x=>!hidden(x)),listed=o.hide?prs:shown,tab=o.tabs[at]||{},inbox=tab.kind==='inbox',hit=(r,x)=>r?v(r.kind,r.i,x):'n',g=new Grid(COLS,o.all?40:LINES);
  const cols=o.cols.filter(c=>c!=='why'||prs.some(z=>z.why)),W={...CW,title:Math.max(10,COLS-1-LEAD-cols.filter(c=>c!=='title').reduce((n,c)=>n+CW[c]+2,0)-2)},tabBg=mix(p.base,p.blue,p.dark?.35:.25),pick=mix(p.base,p.mauve,p.dark?.2:.14);
  // With a scope set, the scope switch takes the viewer's place, as wide as the longest value (at most 24 cells), on all.
  const sv=z=>o.scope==='owner'?z.repo.split('/')[0]:o.scope==='repo'?z.repo:z.author,sw2=o.scope?Math.min(24,Math.max(3,...prs.map(z=>sw(sv(z))))):0;
  let x=g.put(0,0,[['desk',p.blue,p.surface0,1],[' // ',p.dim],...(o.scope?[['‹',p.dim],[' '],['all'.padEnd(sw2),p.subtext0],[' '],['›',p.dim]]:[[`@${o.viewer||'octocat'}`,p.overlay]]),['   ']]);
  g.tabs=[];o.tabs.forEach((t,i)=>{const n=i===at?shown.length:o.count?o.count(t):'?',from=x;x=g.put(x,0,i===at?[[` ${i+1} `,p.subtext0,tabBg],[`${t.name||'inbox'} (${n}) `,p.text,tabBg,1]]:[[`${i+1} `,p.dim],[t.name||'inbox',p.subtext0],[` (${n})`,p.dim]]);g.tabs.push({i,name:t.name||'inbox',x:from,w:x-from});x=g.put(x,0,[[i===at?'  ':'   ']])});
  const q=r=>prs.filter(z=>hit(r,z)==='?').length,note=[...(o.mark?[`${shown.filter(z=>hit(o.mark,z)==='y').length} selected`+(q(o.mark)?`, ${q(o.mark)} ?`:'')]:o.hide?[`${prs.filter(z=>hit(o.hide,z)==='y').length} excluded`+(q(o.hide)?`, ${q(o.hide)} ?`:'')]:[])].join(' · ')+(o.spin?` ${o.spin}`:'');
  g.put(COLS-sw(note),0,[[note,o.mark?p.mauve:p.subtext0]]);
  const head=q=>({ci:'CI'}[q]||q.toUpperCase());let cx=LEAD;for(const q of cols){g.put(cx,1,[[q==='age'?head(q).padStart(W[q]):fit(head(q),W[q]),p.subtext1,null,1]]);cx+=W[q]+2}
  const cell=(q,z,dim)=>{const d=v=>dim?p.dim:v;switch(q){
    case'pr':{const own=z.repo.split('/')[0],r=o.owners.includes(own)?z.repo.split('/')[1]:z.repo,num='#'+z.n,n=z.draft||dim;return[[r,n?p.dim:p.subtext1],[num,n?p.dim:p.overlay]]}
    case'title':{const done=z.state==='merged'||z.state==='closed';return[[fit(done?`${z.title} ${z.state}`:z.title,W.title),z.draft||done?p.dim:d(p.text)]]}case'author':return[[fit(z.author,W.author),d(p.subtext1)]];
    case'ci':{if(!z.heavy)return[['⋅',p.dim]];const m={passing:['✓',p.green],failing:['✗',p.red],pending:['●',p.yellow]}[z.checks]||['·',p.dim];return[[m[0],d(m[1])]]}
    case'merge':{if(!z.heavy)return[['⋅',p.dim]];if(z.state!=='open')return[['']];const m={dirty:['✗',p.red],behind:['↓',p.yellow],unknown:['·',p.dim]}[z.merge];return m?[[m[0],d(m[1])]]:[['']]}
    case'review':{if(!z.heavy)return[['⋅',p.dim]];const m={approved:['✓',p.green],changes_requested:['±',p.red],review_required:['○',p.overlay],none:['·',p.dim]}[z.decision];return[[m[0],d(m[1])],...(z.review?[[' '],[z.review,d(p.subtext1)]]:[])]}
    case'why':return[[fit(z.why,W.why),d(p.peach)]];
    case'size':{if(!z.heavy)return[['⋅',p.dim]];const a='+'+z.add,b='−'+z.del;return[[a,d(p.green)],[' '],[b,d(p.red)]]}
    case'age':return[[z.age.padStart(W.age),o.stale!=='off'&&z.updated>=o.stale?p.yellow:d(p.dim)]]}};
  let y=2,first=true,cur=null;const room=g.h-1;
  const row=z=>{if(y>=room)return;const mv=hit(o.mark,z),hv=hit(o.hide,z),m=mv==='y',h=o.hide&&(hv==='y'||hidden(z)),on=first;if(on)cur=z;first=false;const mk=m?['✓',p.mauve]:h?['⊘',p.dim]:mv==='?'||hv==='?'?['?',p.yellow]:null;g.put(0,y,[[on?'▎':' ',p.blue],[' '],[mk?mk[0]:' ',mk?mk[1]:null]]);let cx=LEAD;for(const q of cols){g.put(cx,y,cell(q,z,h));cx+=W[q]+2}if(on||m)g.band(y,m?pick:p.surface0);y++};
  const newest=(a,b)=>b.at-a.at;
  if(!o.prs){g.put(LEAD,3,[['Not read yet. gh-desk reads this tab the next time it runs.',p.overlay]])}
  // The rows under the heads of the sections they go in, as gh-desk lays them out; a section with no name has no head, and an excluded one shows only with the excluded.
  if(lay)lay.names.forEach((name,k)=>{if(lay.show[k]==='excluded'&&!o.hide)return;const rs=listed.filter(z=>placed(z)===k).sort(newest);if(!rs.length||y>=room)return;if(name)g.put(0,y++,[['  ▾ ',p.subtext0],[name,p.subtext0,null,1],[' '],[String(rs.length),p.dim]]);rs.forEach(row)})
  else listed.slice().sort(newest).forEach(row);
  if(o.all)return g;
  const k=o.key||(()=>'');status(g,g.h-1,p,[[k('menu')||'enter','actions'],[k('select')||'space','select'],[k('select_rule')||'S','select by rule'],[k('triage')||'C','triage with claude'],[k('log')||'L','log'],[k('filter')||'/','filter'],[k('help')||'?','keys']]);
  const s=o.scene&&SCENES[o.scene];if(s&&(cur||prs[0]))s(g,{...o,v,cur:cur||prs[0],shown});return g}
