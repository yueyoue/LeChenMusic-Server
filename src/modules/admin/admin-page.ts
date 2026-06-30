export const adminPageHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>LeChenMusic 管理后台</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{--bg:#0f172a;--card:#1e293b;--border:#334155;--text:#e2e8f0;--muted:#94a3b8;--primary:#3b82f6;--danger:#ef4444;--success:#22c55e}
body{font-family:-apple-system,system-ui,sans-serif;background:var(--bg);color:var(--text);display:flex;min-height:100vh}
.sidebar{width:220px;background:var(--card);border-right:1px solid var(--border);padding:16px 0;flex-shrink:0}
.sidebar h2{padding:0 16px 16px;font-size:18px;border-bottom:1px solid var(--border);margin-bottom:8px}
.sidebar a{display:block;padding:10px 16px;color:var(--muted);text-decoration:none;font-size:14px;cursor:pointer}
.sidebar a:hover,.sidebar a.active{color:var(--text);background:rgba(59,130,246,.1);border-right:3px solid var(--primary)}
.main{flex:1;padding:24px;overflow-y:auto}
.header{display:flex;justify-content:space-between;align-items:center;margin-bottom:24px}
.header h1{font-size:22px}
.btn{padding:8px 16px;border:none;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500}
.btn-primary{background:var(--primary);color:#fff}
.btn-danger{background:var(--danger);color:#fff}
.btn-sm{padding:4px 10px;font-size:12px}
.btn:hover{opacity:.85}
table{width:100%;border-collapse:collapse;background:var(--card);border-radius:8px;overflow:hidden}
th,td{padding:10px 12px;text-align:left;border-bottom:1px solid var(--border);font-size:13px}
th{background:#0f172a;color:var(--muted);font-weight:500;font-size:12px;text-transform:uppercase}
input,select{background:var(--card);border:1px solid var(--border);color:var(--text);padding:8px 12px;border-radius:6px;font-size:13px;outline:none}
input:focus{border-color:var(--primary)}
.modal-overlay{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.6);display:flex;justify-content:center;align-items:center;z-index:100}
.modal{background:var(--card);border-radius:12px;padding:24px;width:420px;max-width:90vw}
.modal h3{margin-bottom:16px}
.modal .form-group{margin-bottom:12px}
.modal label{display:block;font-size:12px;color:var(--muted);margin-bottom:4px}
.modal input,.modal select,.modal textarea{width:100%}
.modal textarea{min-height:80px;resize:vertical}
.modal .actions{display:flex;gap:8px;justify-content:flex-end;margin-top:16px}
.msg{padding:8px 12px;border-radius:6px;font-size:13px;margin-bottom:12px;display:none}
.msg.show{display:block}
.msg.ok{background:#064e3b;color:#6ee7b7}
.msg.err{background:#7f1d1d;color:#fca5a5}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:500}
.badge-admin{background:#7c3aed;color:#fff}
.badge-user{background:#334155;color:#94a3b8}
.badge-idle{background:#064e3b;color:#6ee7b7}
.badge-scanning{background:#854d0e;color:#fbbf24}
.badge-error{background:#7f1d1d;color:#fca5a5}
.search-bar{display:flex;gap:8px;margin-bottom:16px}
.search-bar input{flex:1}
.pager{display:flex;gap:8px;justify-content:center;margin-top:16px;align-items:center}
.pager button{padding:6px 12px}
.pager span{font-size:13px;color:var(--muted)}
@media(max-width:768px){.sidebar{width:60px}.sidebar h2,.sidebar a span{display:none}.sidebar a{text-align:center;padding:12px 0}}
</style>
</head>
<body>
<div class="sidebar">
  <h2>🎵 管理后台</h2>
  <a onclick="nav('dashboard')" id="nav-dashboard"><span>📊 仪表盘</span></a>
  <a onclick="nav('libraries')" id="nav-libraries"><span>📁 媒体库</span></a>
  <a onclick="nav('tracks')" id="nav-tracks"><span>🎵 歌曲</span></a>
  <a onclick="nav('albums')" id="nav-albums"><span>💿 专辑</span></a>
  <a onclick="nav('artists')" id="nav-artists"><span>🎤 艺人</span></a>
  <a onclick="nav('audiobooks')" id="nav-audiobooks"><span>📖 有声书</span></a>
  <a onclick="nav('users')" id="nav-users"><span>👥 用户</span></a>
  <a href="/" style="margin-top:auto"><span>🏠 返回首页</span></a>
</div>
<div class="main">
  <div id="msg" class="msg"></div>
  <div id="content"></div>
</div>

<script>
const API = '';
let token = localStorage.getItem('token');
if (!token) location.href = '/';

function headers() { return { 'Content-Type': 'application/json', 'Authorization': '*** ' + token }; }
function showMsg(t,ok){const m=document.getElementById('msg');m.textContent=t;m.className='msg show '+(ok?'ok':'err');setTimeout(()=>m.className='msg',3000)}
function fmtDate(d){if(!d)return'-';return new Date(d*1000).toLocaleString('zh-CN')}
function fmtSize(b){if(!b)return'0 B';const k=1024,s=['B','KB','MB','GB','TB'];const i=Math.floor(Math.log(b)/Math.log(k));return(b/Math.pow(k,i)).toFixed(1)+' '+s[i]}
function fmtDur(s){if(!s)return'-';const m=Math.floor(s/60),sec=s%60;return m+':'+String(sec).padStart(2,'0')}

async function api(path,opts){
  const res=await fetch(API+path,{headers:headers(),...opts});
  const data=await res.json();
  if(data.code!==0&&data.message)showMsg(data.message,false);
  return data;
}

let currentPage='dashboard';
function nav(page){
  currentPage=page;
  document.querySelectorAll('.sidebar a').forEach(a=>a.classList.remove('active'));
  const el=document.getElementById('nav-'+page);if(el)el.classList.add('active');
  window['render_'+page]();
}

// ─── 仪表盘 ──────────────────────────────────────────
async function render_dashboard(){
  const r=await api('/api/admin/dashboard');
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>📊 仪表盘</h1></div>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px">
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${d.tracks}</div><div style="color:var(--muted);font-size:13px">🎵 歌曲</div>
      </div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${d.albums}</div><div style="color:var(--muted);font-size:13px">💿 专辑</div>
      </div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${d.artists}</div><div style="color:var(--muted);font-size:13px">🎤 艺人</div>
      </div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${d.users}</div><div style="color:var(--muted);font-size:13px">👥 用户</div>
      </div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${d.libraries}</div><div style="color:var(--muted);font-size:13px">📁 媒体库</div>
      </div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center">
        <div style="font-size:28px;font-weight:700">\${fmtSize(d.totalSizeBytes)}</div><div style="color:var(--muted);font-size:13px">💾 总大小</div>
      </div>
    </div>\`;
}

// ─── 媒体库 ──────────────────────────────────────────
async function render_libraries(){
  const r=await api('/api/admin/libraries');
  if(r.code!==0)return;
  const items=r.data||[];
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>📁 媒体库管理</h1><button class="btn btn-primary" onclick="showAddLibrary()">+ 添加媒体库</button></div>
    <table><thead><tr><th>ID</th><th>名称</th><th>类型</th><th>路径</th><th>状态</th><th>文件数</th><th>上次扫描</th><th>操作</th></tr></thead>
    <tbody>\${items.map(l=>\`<tr>
      <td>\${l.id}</td><td>\${l.name}</td><td>\${l.storageType}</td><td style="max-width:200px;overflow:hidden;text-overflow:ellipsis">\${l.storagePath}</td>
      <td><span class="badge badge-\${l.scanStatus}">\${l.scanStatus}</span></td><td>\${l.fileCount||0}</td><td>\${fmtDate(l.lastScanAt)}</td>
      <td><button class="btn btn-primary btn-sm" onclick="scanLib(\${l.id})">扫描</button> <button class="btn btn-danger btn-sm" onclick="deleteLib(\${l.id})">删除</button></td>
    </tr>\`).join('')}</tbody></table>
    <div id="libModal"></div>\`;
}

function showAddLibrary(){
  document.getElementById('libModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>添加媒体库</h3>
        <div class="form-group"><label>名称</label><input id="libName" placeholder="如：我的音乐"></div>
        <div class="form-group"><label>存储类型</label><select id="libType"><option value="local">本地磁盘</option><option value="smb">SMB/NFS</option></select></div>
        <div class="form-group"><label>路径（容器内路径）</label><input id="libPath" placeholder="/music"></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="addLibrary()">添加</button></div>
      </div>
    </div>\`;
}

async function addLibrary(){
  const name=document.getElementById('libName').value;
  const storageType=document.getElementById('libType').value;
  const storagePath=document.getElementById('libPath').value;
  if(!name||!storagePath)return showMsg('请填写完整',false);
  const r=await api('/api/admin/libraries',{method:'POST',body:JSON.stringify({name,storageType,storagePath})});
  if(r.code===0){showMsg('添加成功',true);document.querySelector('.modal-overlay').remove();render_libraries();}
}

async function scanLib(id){
  const r=await api('/api/admin/libraries/'+id+'/scan',{method:'POST'});
  if(r.code===0)showMsg('扫描已启动，请稍候...',true);
}

async function deleteLib(id){
  if(!confirm('确定删除此媒体库？'))return;
  const r=await api('/api/admin/libraries/'+id,{method:'DELETE'});
  if(r.code===0){showMsg('已删除',true);render_libraries();}
}

// ─── 歌曲 ────────────────────────────────────────────
let trackPage=1;
async function render_tracks(){
  const search=document.getElementById('trackSearch')?.value||'';
  const r=await api('/api/admin/tracks?page='+trackPage+'&pageSize=30&search='+encodeURIComponent(search));
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>🎵 歌曲管理</h1><span style="color:var(--muted)">共 \${d.total} 首</span></div>
    <div class="search-bar"><input id="trackSearch" placeholder="搜索歌曲名、流派..." value="\${search}" onkeydown="if(event.key==='Enter'){trackPage=1;render_tracks()}"><button class="btn btn-primary" onclick="trackPage=1;render_tracks()">搜索</button></div>
    <table><thead><tr><th>ID</th><th>歌曲名</th><th>艺人</th><th>专辑</th><th>时长</th><th>格式</th><th>码率</th><th>流派</th><th>操作</th></tr></thead>
    <tbody>\${d.items.map(t=>\`<tr>
      <td>\${t.id}</td><td>\${t.title}</td><td>\${t.artistName||'-'}</td><td>\${t.albumTitle||'-'}</td>
      <td>\${fmtDur(t.duration)}</td><td>\${t.format||'-'}</td><td>\${t.bitrate?t.bitrate+'kbps':'-'}</td><td>\${t.genre||'-'}</td>
      <td><button class="btn btn-primary btn-sm" onclick="editTrack(\${t.id},'\${t.title?.replace(/'/g,"\\\\'")}','\${t.genre||''}',\${t.trackNumber||0},\${t.discNumber||1})">编辑</button></td>
    </tr>\`).join('')}</tbody></table>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(trackPage>1){trackPage--;render_tracks()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){trackPage++;render_tracks()}" \${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="trackModal"></div>\`;
}

function editTrack(id,title,genre,trackNo,discNo){
  document.getElementById('trackModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑歌曲 #\${id}</h3>
        <div class="form-group"><label>歌曲名</label><input id="etTitle" value="\${title}"></div>
        <div class="form-group"><label>流派</label><input id="etGenre" value="\${genre}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>曲目号</label><input id="etTrack" type="number" value="\${trackNo}"></div>
          <div class="form-group" style="flex:1"><label>碟片号</label><input id="etDisc" type="number" value="\${discNo}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveTrack(\${id})">保存</button></div>
      </div>
    </div>\`;
}

async function saveTrack(id){
  const r=await api('/api/admin/tracks/'+id,{method:'PUT',body:JSON.stringify({
    title:document.getElementById('etTitle').value,
    genre:document.getElementById('etGenre').value,
    trackNumber:parseInt(document.getElementById('etTrack').value)||null,
    discNumber:parseInt(document.getElementById('etDisc').value)||1,
  })});
  if(r.code===0){showMsg('保存成功',true);document.querySelector('.modal-overlay').remove();render_tracks();}
}

// ─── 专辑 ────────────────────────────────────────────
let albumPage=1;
async function render_albums(){
  const search=document.getElementById('albumSearch')?.value||'';
  const r=await api('/api/admin/albums?page='+albumPage+'&pageSize=30&search='+encodeURIComponent(search));
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>💿 专辑管理</h1><span style="color:var(--muted)">共 \${d.total} 张</span></div>
    <div class="search-bar"><input id="albumSearch" placeholder="搜索专辑名..." value="\${search}" onkeydown="if(event.key==='Enter'){albumPage=1;render_albums()}"><button class="btn btn-primary" onclick="albumPage=1;render_albums()">搜索</button></div>
    <table><thead><tr><th>ID</th><th>专辑名</th><th>艺人</th><th>年份</th><th>流派</th><th>操作</th></tr></thead>
    <tbody>\${d.items.map(a=>\`<tr>
      <td>\${a.id}</td><td>\${a.title}</td><td>\${a.artistName||'-'}</td><td>\${a.year||'-'}</td><td>\${a.genre||'-'}</td>
      <td><button class="btn btn-primary btn-sm" onclick="editAlbum(\${a.id},'\${a.title?.replace(/'/g,"\\\\'")}',\${a.year||0},'\${a.genre||''}')">编辑</button></td>
    </tr>\`).join('')}</tbody></table>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(albumPage>1){albumPage--;render_albums()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){albumPage++;render_albums()}" \${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="albumModal"></div>\`;
}

function editAlbum(id,title,year,genre){
  document.getElementById('albumModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑专辑 #\${id}</h3>
        <div class="form-group"><label>专辑名</label><input id="eaTitle" value="\${title}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>年份</label><input id="eaYear" type="number" value="\${year}"></div>
          <div class="form-group" style="flex:1"><label>流派</label><input id="eaGenre" value="\${genre}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveAlbum(\${id})">保存</button></div>
      </div>
    </div>\`;
}

async function saveAlbum(id){
  const r=await api('/api/admin/albums/'+id,{method:'PUT',body:JSON.stringify({
    title:document.getElementById('eaTitle').value,
    year:parseInt(document.getElementById('eaYear').value)||null,
    genre:document.getElementById('eaGenre').value,
  })});
  if(r.code===0){showMsg('保存成功',true);document.querySelector('.modal-overlay').remove();render_albums();}
}

// ─── 艺人 ────────────────────────────────────────────
let artistPage=1;
async function render_artists(){
  const search=document.getElementById('artistSearch')?.value||'';
  const r=await api('/api/admin/artists?page='+artistPage+'&pageSize=30&search='+encodeURIComponent(search));
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>🎤 艺人管理</h1><span style="color:var(--muted)">共 \${d.total} 位</span></div>
    <div class="search-bar"><input id="artistSearch" placeholder="搜索艺人名..." value="\${search}" onkeydown="if(event.key==='Enter'){artistPage=1;render_artists()}"><button class="btn btn-primary" onclick="artistPage=1;render_artists()">搜索</button></div>
    <table><thead><tr><th>ID</th><th>艺人名</th><th>简介</th><th>操作</th></tr></thead>
    <tbody>\${d.items.map(a=>\`<tr>
      <td>\${a.id}</td><td>\${a.name}</td><td style="max-width:300px;overflow:hidden;text-overflow:ellipsis">\${a.bio||'-'}</td>
      <td><button class="btn btn-primary btn-sm" onclick="editArtist(\${a.id},'\${a.name?.replace(/'/g,"\\\\'")}','\${(a.bio||'').replace(/'/g,"\\\\'").replace(/\\n/g,'\\\\n')}')">编辑</button></td>
    </tr>\`).join('')}</tbody></table>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(artistPage>1){artistPage--;render_artists()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){artistPage++;render_artists()}" \${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="artistModal"></div>\`;
}

function editArtist(id,name,bio){
  document.getElementById('artistModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑艺人 #\${id}</h3>
        <div class="form-group"><label>艺人名</label><input id="eaName" value="\${name}"></div>
        <div class="form-group"><label>简介</label><textarea id="eaBio">\${bio.replace(/\\\\n/g,'\\n')}</textarea></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveArtist(\${id})">保存</button></div>
      </div>
    </div>\`;
}

async function saveArtist(id){
  const r=await api('/api/admin/artists/'+id,{method:'PUT',body:JSON.stringify({
    name:document.getElementById('eaName').value,
    bio:document.getElementById('eaBio').value,
  })});
  if(r.code===0){showMsg('保存成功',true);document.querySelector('.modal-overlay').remove();render_artists();}
}

// ─── 用户 ────────────────────────────────────────────
async function render_users(){
  const r=await api('/api/admin/users');
  if(r.code!==0)return;
  const items=r.data||[];
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>👥 用户管理</h1><button class="btn btn-primary" onclick="showAddUser()">+ 添加用户</button></div>
    <table><thead><tr><th>ID</th><th>用户名</th><th>昵称</th><th>角色</th><th>注册时间</th><th>操作</th></tr></thead>
    <tbody>\${items.map(u=>\`<tr>
      <td>\${u.id}</td><td>\${u.username}</td><td>\${u.displayName||'-'}</td>
      <td><span class="badge badge-\${u.role}">\${u.role==='admin'?'👑 管理员':'👤 用户'}</span></td>
      <td>\${fmtDate(u.createdAt)}</td>
      <td>\${u.role!=='admin'? \`<button class="btn btn-primary btn-sm" onclick="setAdmin(\${u.id})">设为管理员</button>\` : ''}
          <button class="btn btn-danger btn-sm" onclick="deleteUser(\${u.id})">删除</button></td>
    </tr>\`).join('')}</tbody></table>
    <div id="userModal"></div>\`;
}

function showAddUser(){
  document.getElementById('userModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>添加用户</h3>
        <div class="form-group"><label>用户名</label><input id="nuUser" placeholder="至少3位"></div>
        <div class="form-group"><label>昵称</label><input id="nuName"></div>
        <div class="form-group"><label>密码</label><input id="nuPass" type="password" placeholder="至少6位"></div>
        <div class="form-group"><label>角色</label><select id="nuRole"><option value="user">普通用户</option><option value="admin">管理员</option></select></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="addUser()">添加</button></div>
      </div>
    </div>\`;
}

async function addUser(){
  const username=document.getElementById('nuUser').value;
  const password=document.getElementById('nuPass').value;
  if(!username||!password)return showMsg('请填写用户名和密码',false);
  const r=await api('/api/admin/users',{method:'POST',body:JSON.stringify({
    username,password,
    displayName:document.getElementById('nuName').value||undefined,
    role:document.getElementById('nuRole').value,
  })});
  if(r.code===0){showMsg('添加成功',true);document.querySelector('.modal-overlay').remove();render_users();}
}

async function setAdmin(id){
  if(!confirm('确定将此用户设为管理员？'))return;
  const r=await api('/api/admin/users/'+id,{method:'PUT',body:JSON.stringify({role:'admin'})});
  if(r.code===0){showMsg('已设为管理员',true);render_users();}
}

async function deleteUser(id){
  if(!confirm('确定删除此用户？'))return;
  const r=await api('/api/admin/users/'+id,{method:'DELETE'});
  if(r.code===0){showMsg('已删除',true);render_users();}
}


// ─── 有声书 ────────────────────────────────────────────
let abPage=1;
async function render_audiobooks(){
  var search=(document.getElementById('abSearch')||{}).value||'';
  var r=await api('/api/admin/audiobooks?page='+abPage+'&pageSize=30&search='+encodeURIComponent(search));
  if(r.code!==0)return;
  var d=r.data;
  var html='<div class="header"><h1>📖 有声书管理</h1><span style="color:var(--muted)">共 '+d.total+' 本</span></div>';
  html+='<div class="search-bar"><input id="abSearch" placeholder="搜索有声书名..." value="'+search+'" onkeydown="if(event.key===\'Enter\'){abPage=1;render_audiobooks()}"><button class="btn btn-primary" onclick="abPage=1;render_audiobooks()">搜索</button></div>';
  html+='<table><thead><tr><th>ID</th><th>书名</th><th>作者</th><th>演播</th><th>分类</th><th>章节</th><th>总时长</th><th>操作</th></tr></thead><tbody>';
  for(var i=0;i<d.items.length;i++){
    var a=d.items[i];
    html+='<tr><td>'+a.id+'</td><td>'+a.title+'</td><td>'+(a.author||'-')+'</td><td>'+(a.narrator||'-')+'</td>';
    html+='<td>'+(a.genre||'-')+'</td><td>'+(a.chapterCount||0)+'</td><td>'+fmtDur(a.totalDuration)+'</td>';
    html+='<td><button class="btn btn-primary btn-sm" onclick="editAudiobook('+a.id+',\''+a.title?.replace(/'/g,"\\'")+'\',\''+(a.author||'')+'\',\''+(a.narrator||'')+'\',\''+(a.genre||'')+'\','+(a.year||0)+')">编辑</button> ';
    html+='<button class="btn btn-danger btn-sm" onclick="deleteAudiobook('+a.id+')">删除</button></td></tr>';
  }
  html+='</tbody></table>';
  html+='<div class="pager">';
  html+='<button class="btn btn-sm" onclick="if(abPage>1){abPage--;render_audiobooks()}" '+(d.page<=1?'disabled':'')+'>上一页</button>';
  html+='<span>第 '+d.page+' / '+Math.ceil(d.total/d.pageSize)+' 页</span>';
  html+='<button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){abPage++;render_audiobooks()}" '+(d.page*d.pageSize>=d.total?'disabled':'')+'>下一页</button>';
  html+='</div><div id="abModal"></div>';
  document.getElementById('content').innerHTML=html;
}

function editAudiobook(id,title,author,narrator,genre,year){
  var html='<div class="modal-overlay" onclick="if(event.target===this)this.remove()">';
  html+='<div class="modal"><h3>编辑有声书 #'+id+'</h3>';
  html+='<div class="form-group"><label>书名</label><input id="eabTitle" value="'+title+'"></div>';
  html+='<div style="display:flex;gap:8px">';
  html+='<div class="form-group" style="flex:1"><label>作者</label><input id="eabAuthor" value="'+author+'"></div>';
  html+='<div class="form-group" style="flex:1"><label>演播者</label><input id="eabNarrator" value="'+narrator+'"></div>';
  html+='</div>';
  html+='<div style="display:flex;gap:8px">';
  html+='<div class="form-group" style="flex:1"><label>分类</label><input id="eabGenre" value="'+genre+'"></div>';
  html+='<div class="form-group" style="flex:1"><label>年份</label><input id="eabYear" type="number" value="'+year+'"></div>';
  html+='</div>';
  html+='<div class="actions"><button class="btn" onclick="this.closest(\'.modal-overlay\').remove()">取消</button>';
  html+='<button class="btn btn-primary" onclick="saveAudiobook('+id+')">保存</button></div></div></div>';
  document.getElementById('abModal').innerHTML=html;
}

async function saveAudiobook(id){
  var r=await api('/api/admin/audiobooks/'+id,{method:'PUT',body:JSON.stringify({
    title:document.getElementById('eabTitle').value,
    author:document.getElementById('eabAuthor').value,
    narrator:document.getElementById('eabNarrator').value,
    genre:document.getElementById('eabGenre').value,
    year:parseInt(document.getElementById('eabYear').value)||null
  })});
  if(r.code===0){showMsg('保存成功',true);document.querySelector('.modal-overlay').remove();render_audiobooks();}
}

async function deleteAudiobook(id){
  if(!confirm('确定删除此有声书？'))return;
  var r=await api('/api/admin/audiobooks/'+id,{method:'DELETE'});
  if(r.code===0){showMsg('已删除',true);render_audiobooks();}
}

// 初始化
nav('dashboard');
</script>
</body></html>`;
