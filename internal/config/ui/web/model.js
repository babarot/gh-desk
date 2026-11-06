// Pure: what the page knows of the settings. No DOM, no Preact.
const same=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
const clone=v=>v===undefined?undefined:JSON.parse(JSON.stringify(v));
const ARR=['tabs','select','exclude','auto_approve'];
const settingOf=k=>DATA.settings.find(s=>s.key===k);
const defaultOf=s=>s.defv;
const defaultRules=()=>clone(DATA.defaultRules);
const defaultTabs=()=>clone(DATA.defaultTabs);
function tomlStr(s){if(/\\/.test(s)&&!/['\n\r\t]/.test(s))return`'${s}'`;return JSON.stringify(s)}
const tomlKey=k=>/^[A-Za-z0-9_-]+$/.test(k)?k:JSON.stringify(k);
function tomlVal(v){if(typeof v==='string')return tomlStr(v);if(typeof v!=='object')return String(v);if(Array.isArray(v))return'['+v.map(tomlVal).join(', ')+']';const es=Object.entries(v);return es.length?'{ '+es.map(([k,x])=>`${tomlKey(k)} = ${tomlVal(x)}`).join(', ')+' }':'{}'}
// load takes the file as Go read it: every key written, and nothing more.
function load(t){const d={vals:{},arr:{},keys:{},comments:t.__comments||0};
  for(const s of DATA.settings){let o=t,ok=true;for(const p of s.key.split('.')){if(o&&typeof o==='object'&&!Array.isArray(o)&&p in o)o=o[p];else{ok=false;break}}if(ok)d.vals[s.key]=o}
  for(const a of ARR)if(a in t)d.arr[a]=t[a];Object.assign(d.keys,t.keys||{});return d}
function keyDefault(a){const k=KEYS.find(x=>x.action===a);if(!k||!k.keys)return undefined;return k.keys.length===1?k.keys[0]:k.keys}
function genToml(d){const out=['#:schema '+DATA.schema,''];
  const wr=(n,rows)=>{for(const r of rows){out.push(`[[${n}]]`);for(const[k,v]of Object.entries(r))out.push(`${k} = ${tomlVal(v)}`);out.push('')}};
  if(d.arr.select&&!d.arr.select.length)out.push('select = []','');
  for(const n of ARR)if(d.arr[n]&&d.arr[n].length)wr(n,d.arr[n]);
  for(const t of DATA.tables){const ls=DATA.settings.filter(s=>s.table===t.name&&s.key in d.vals).map(s=>`${s.name} = ${tomlVal(d.vals[s.key])}`);if(ls.length)out.push(`[${t.name}]`,...ls,'')}
  const ks=Object.entries(d.keys);if(ks.length)out.push('[keys]',...ks.map(([a,v])=>`${a} = ${tomlVal(v)}`),'');
  return out.join('\n').replace(/\n+$/,'\n')}
function changes(f,d){const r=[];
  for(const s of DATA.settings){const a=f.vals[s.key],b=d.vals[s.key];if(!same(a,b))r.push({id:s.key,type:'val',from:a,to:b})}
  for(const n of ARR){const a=f.arr[n],b=d.arr[n];if(!same(a,b))r.push({id:n,type:'arr',from:a,to:b})}
  for(const a of new Set([...Object.keys(f.keys),...Object.keys(d.keys)]))if(!same(f.keys[a],d.keys[a]))r.push({id:a,type:'key',from:f.keys[a],to:d.keys[a]});
  return r}
// One value of a list, as internal/config checks it.
function checkItem(type,v){
  if(type==='repo'&&!/^[^\s/]+\/[^\s/]+$/.test(v))return{msg:`${v} must be owner/repo.`};
  if(type==='glob'){let open=false;for(let i=0;i<v.length;i++){if(v[i]==='\\'){i++;continue}if(v[i]==='[')open=true;if(v[i]===']')open=false}if(open||/\\$/.test(v))return{msg:`${v} is not a glob pattern.`}}
  if(type==='author'&&v.startsWith('app/'))return{warn:1,msg:`Write ${v.slice(4)}, without app/.`};
  return null}
const itemType=k=>k==='repos'?'repo':k==='authors'?'author':null;
const searchesPRs=q=>/(^|\s)(is|type):pr(\s|$)/.test(q);
function goRe(s){let f='';s=s.replace(/^\(\?i\)/,()=>{f='i';return''});return new RegExp(s,f)}
function checkValue(k,v){const t=k.type.replace('config.','');
  if(t==='intKind'&&(typeof v!=='number'||Number.isNaN(v)||v<k.min||(k.max&&v>k.max)))return`Enter a ${k.name}.`;
  if(t==='durationKind'&&!(/^(\d+(\.\d+)?(ns|us|ms|s|m|h))+$/.test(v)||(k.off&&v==='off')))return'Enter a time such as 3m or 90s.';
  if(t==='daysKind'&&!(v==='off'||(Number.isInteger(v)&&v>0)))return'Enter a whole number of days.';
  if(t==='stringKind'&&k.nonEmpty&&!v)return'This cannot be empty.';
  if(t==='listKind'&&k.nonEmpty&&!v.length)return'Choose at least one.';
  return''}
function problems(d){const ps=[];
  for(const s of DATA.settings)if(s.key in d.vals){const m=checkValue(s.kind,d.vals[s.key]);if(m)ps.push({at:s.key,msg:m});if(s.kind.type.endsWith('globsKind'))for(const v of d.vals[s.key]){const c=checkItem('glob',v);if(c&&!c.warn)ps.push({at:s.key,msg:c.msg})}}
  const rules=(n,named)=>{const seen={};(d.arr[n]||[]).forEach((r,i)=>{const at=`${n}/${i}`;
    if(named&&!r.name)ps.push({at,msg:'Give this rule a name.'});
    if(r.name){if(seen[r.name])ps.push({at,msg:`The name “${r.name}” is used twice.`});seen[r.name]=1}
    for(const k of ['repos','authors'])for(const v of r[k]||[]){const c=checkItem(itemType(k),v);if(c&&!c.warn)ps.push({at:`${at}/${k}`,msg:c.msg})}
    if(r.title!==undefined){try{goRe(r.title)}catch(e){ps.push({at,msg:'The title pattern is not a valid regular expression.'})}}
    if(!Object.keys(r).some(k=>!['name','query','dry_run'].includes(k))&&!(n==='auto_approve'&&r.query))ps.push({at,msg:'Add a condition, or this rule matches every pull request.'})})};
  rules('select',true);rules('exclude',false);rules('auto_approve',false);
  if(d.arr.tabs){const t=d.arr.tabs;if(t.filter(x=>x.kind==='inbox').length!==1)ps.push({at:'tabs',msg:'Keep one inbox tab.'});if(t.length>9)ps.push({at:'tabs',msg:'Keep nine tabs or fewer.'});t.forEach((x,i)=>{if(x.kind!=='inbox'&&!x.query)ps.push({at:`tabs/${i}`,msg:'A tab needs a search.'})})}
  const seen={};for(const k of KEYS){const v=d.keys[k.action];if(!v)continue;for(const key of[].concat(v)){const id=k.scope+'/'+key;if(seen[id])ps.push({at:'key/'+k.action,msg:`${key} is also set for ${seen[id].replace(/_/g,' ')}.`});seen[id]=k.action}}
  return ps}
function matchRule(r,p){for(const[k,v]of Object.entries(r))switch(k){
  case'authors':if(!v.includes(p.author))return false;break;
  case'repos':if(!v.some(g=>new RegExp('^'+g.replace(/[.+]/g,'\\$&').replace(/\*/g,'[^/]*')+'$').test(p.repo)))return false;break;
  case'title':try{if(!goRe(v).test(p.title))return false}catch(e){return false}break;
  case'max_files':if(p.files>v)return false;break;case'min_files':if(p.files<v)return false;break;
  case'max_lines':if(p.lines>v)return false;break;case'min_lines':if(p.lines<v)return false;break;
  case'updated_within_days':if(p.updated>v)return false;break;case'created_within_days':if(p.created>v)return false;break;
  case'updated_before_days':if(p.updated<v)return false;break;case'created_before_days':if(p.created<v)return false;break;
  case'bot':case'mine':case'draft':case'approved_by_me':case'reviewed_by_me':if(!!p[k]!==v)return false;break;
  case'state':if(v.length&&!v.includes(p.state))return false;break;case'checks':if(v.length&&!v.includes(p.checks))return false;break;
  case'decision':if(v.length&&!v.includes(p.decision))return false;break;
  case'labels':if(v.length&&!v.some(l=>p.labels.includes(l)))return false;break;
  case'requested':if(v.length&&!v.some(x=>p.requested.includes(x)))return false;break}
  return true}
