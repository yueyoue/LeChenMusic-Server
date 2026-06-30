const API = '';
let token = localStorage.getItem('token');
if (!token) location.href = '/';

function headers() { return { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token }; }
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
  document.querySelectorAll('.sidebar .nav-item').forEach(a=>a.classList.remove('active'));
  const el=document.getElementById('nav-'+page);if(el)el.classList.add('active');
  window['render_'+page]();
}

// ─── 仪表盘 ──────────────────────────────────────────
async function render_dashboard(){
  const r=await api('/api/admin/dashboard');
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=`
    <div class="header"><h1>📊 仪表盘</h1></div>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px">
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${d.tracks}</div><div style="color:var(--muted);font-size:13px">🎵 歌曲</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${d.albums}</div><div style="color:var(--muted);font-size:13px">💿 专辑</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${d.artists}</div><div style="color:var(--muted);font-size:13px">🎤 艺人</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${d.users}</div><div style="color:var(--muted);font-size:13px">👥 用户</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${d.libraries}</div><div style="color:var(--muted);font-size:13px">📁 媒体库</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">${fmtSize(d.totalSizeBytes)}</div><div style="color:var(--muted);font-size:13px">💾 总大小</div></div>
    </div>`;
}

// ─── 媒体库 ──────────────────────────────────────────
async function render_libraries(){
  const r=await api('/api/admin/libraries');
  if(r.code!==0)return;
  const items=r.data||[];
  document.getElementById('content').innerHTML=`
    <div class="header"><h1>📁 媒体库管理</h1><button class="btn btn-primary" onclick="showAddLibrary()">+ 添加媒体库</button></div>
    <table><thead><tr><th>ID</th><th>名称</th><th>类型</th><th>路径</th><th>状态</th><th>文件数</th><th>上次扫描</th><th>操作</th></tr></thead>
    <tbody>${items.map(l=>`<tr>
      <td>${l.id}</td><td>${l.name}</td><td>${l.storageType}</td><td style="max-width:200px;overflow:hidden;text-overflow:ellipsis">${l.storagePath}</td>
      <td><span class="badge badge-${l.scanStatus}">${l.scanStatus}</span></td><td>${l.fileCount||0}</td><td>${fmtDate(l.lastScanAt)}</td>
      <td><button class="btn btn-primary btn-sm" onclick="scanLib(${l.id})">扫描</button> <button class="btn btn-danger btn-sm" onclick="deleteLib(${l.id})">删除</button></td>
    </tr>`).join('')}</tbody></table>
    <div id="libModal"></div>`;
}
function showAddLibrary(){
  document.getElementById('libModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>添加媒体库</h3>
        <div class="form-group"><label>名称</label><input id="libName" placeholder="如：我的音乐"></div>
        <div class="form-group"><label>存储类型</label><select id="libType"><option value="local">本地磁盘</option><option value="smb">SMB/NFS</option></select></div>
        <div class="form-group"><label>路径（容器内路径）</label><input id="libPath" placeholder="/music"></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="addLibrary()">添加</button></div>
      </div>
    </div>`;
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

// ─── 歌曲（表格+排序）────────────────────────────────
let trackPage=1, trackSort='default';
function sortBtns(current,items,onClick){
  return items.map(([k,l])=>`<button class="sort-btn ${current===k?'active':''}" onclick="${onClick}('${k}')">${l}</button>`).join('');
}
async function render_tracks(){
  const search=document.getElementById('trackSearch')?.value||'';
  const r=await api('/api/admin/tracks?page='+trackPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+trackSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=`
    <div class="header">
      <h1>🎵 歌曲 <span style="font-size:14px;color:var(--muted);font-weight:400">共 ${d.total} 首</span></h1>
      <div class="toolbar">
        <input class="search-input" id="trackSearch" placeholder="搜索歌曲名、流派..." value="${search}" onkeydown="if(event.key==='Enter'){trackPage=1;render_tracks()}">
        <div class="sort-btns">${sortBtns(trackSort,[['default','默认排序'],['recent','最近添加'],['plays','最多播放']],'k=>{trackSort=k;trackPage=1;render_tracks()}')}</div>
      </div>
    </div>
    <table>
      <thead><tr><th>歌曲</th><th>艺人</th><th>专辑</th><th>时长</th><th>大小</th><th>操作</th></tr></thead>
      <tbody>${d.items.map(t=>`<tr>
        <td style="font-weight:500">${t.title}</td>
        <td style="color:var(--muted)">${t.artistName||'-'}</td>
        <td style="color:var(--muted)">${t.albumTitle||'-'}</td>
        <td style="color:var(--muted)">${fmtDur(t.duration)}</td>
        <td style="color:var(--muted)">${fmtSize(t.fileSize)}</td>
        <td><button class="btn btn-primary btn-sm" onclick="editTrack(${t.id},'${(t.title||'').replace(/'/g,"\\'")}','${(t.genre||'').replace(/'/g,"\\'")}',${t.trackNumber||0},${t.discNumber||1})">编辑</button></td>
      </tr>`).join('')}
      </tbody>
    </table>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(trackPage>1){trackPage--;render_tracks()}" ${d.page<=1?'disabled':''}>上一页</button>
      <span>第 ${d.page} / ${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){trackPage++;render_tracks()}" ${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="trackModal"></div>`;
}
function editTrack(id,title,genre,trackNo,discNo){
  document.getElementById('trackModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑歌曲 #${id}</h3>
        <div class="form-group"><label>歌曲名</label><input id="etTitle" value="${title}"></div>
        <div class="form-group"><label>流派</label><input id="etGenre" value="${genre}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>曲目号</label><input id="etTrack" type="number" value="${trackNo}"></div>
          <div class="form-group" style="flex:1"><label>碟片号</label><input id="etDisc" type="number" value="${discNo}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveTrack(${id})">保存</button></div>
      </div>
    </div>`;
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

// ─── 专辑（卡片网格+排序）────────────────────────────
let albumPage=1, albumSort='default';
async function render_albums(){
  const search=document.getElementById('albumSearch')?.value||'';
  const r=await api('/api/admin/albums?page='+albumPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+albumSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=`
    <div class="header">
      <h1>💿 专辑 <span style="font-size:14px;color:var(--muted);font-weight:400">共 ${d.total} 张</span></h1>
      <div class="toolbar">
        <input class="search-input" id="albumSearch" placeholder="搜索专辑名..." value="${search}" onkeydown="if(event.key==='Enter'){albumPage=1;render_albums()}">
        <div class="sort-btns">${sortBtns(albumSort,[['default','默认排序'],['recent','最近添加']],'k=>{albumSort=k;albumPage=1;render_albums()}')}</div>
      </div>
    </div>
    <div class="card-grid">
      ${d.items.map(a=>`<div class="card" onclick="editAlbum(${a.id},'${(a.title||'').replace(/'/g,"\\'")}',${a.year||0},'${(a.genre||'').replace(/'/g,"\\'")}')">
        <img class="card-img" src="/api/cover/${a.id}" alt="${a.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><rect fill=%22%23334155%22 width=%22200%22 height=%22200%22/><text fill=%22%2394a3b8%22 font-size=%2248%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.3em%22>💿</text></svg>'">
        <div class="card-body">
          <div class="card-title">${a.title}</div>
          <div class="card-sub">${a.artistName||'未知艺人'}${a.year?' · '+a.year:''}</div>
        </div>
      </div>`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(albumPage>1){albumPage--;render_albums()}" ${d.page<=1?'disabled':''}>上一页</button>
      <span>第 ${d.page} / ${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){albumPage++;render_albums()}" ${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="albumModal"></div>`;
}
function editAlbum(id,title,year,genre){
  document.getElementById('albumModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑专辑 #${id}</h3>
        <div class="form-group"><label>专辑名</label><input id="eaTitle" value="${title}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>年份</label><input id="eaYear" type="number" value="${year}"></div>
          <div class="form-group" style="flex:1"><label>流派</label><input id="eaGenre" value="${genre}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveAlbum(${id})">保存</button></div>
      </div>
    </div>`;
}
async function saveAlbum(id){
  const r=await api('/api/admin/albums/'+id,{method:'PUT',body:JSON.stringify({
    title:document.getElementById('eaTitle').value,
    year:parseInt(document.getElementById('eaYear').value)||null,
    genre:document.getElementById('eaGenre').value,
  })});
  if(r.code===0){showMsg('保存成功',true);document.querySelector('.modal-overlay').remove();render_albums();}
}

// ─── 艺人（圆形头像卡片+排序）────────────────────────
let artistPage=1, artistSort='default';
async function render_artists(){
  const search=document.getElementById('artistSearch')?.value||'';
  const r=await api('/api/admin/artists?page='+artistPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+artistSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=`
    <div class="header">
      <h1>🎤 艺人 <span style="font-size:14px;color:var(--muted);font-weight:400">共 ${d.total} 位</span></h1>
      <div class="toolbar">
        <input class="search-input" id="artistSearch" placeholder="搜索艺人名..." value="${search}" onkeydown="if(event.key==='Enter'){artistPage=1;render_artists()}">
        <div class="sort-btns">${sortBtns(artistSort,[['default','默认排序'],['recent','最近添加']],'k=>{artistSort=k;artistPage=1;render_artists()}')}</div>
      </div>
    </div>
    <div class="card-grid">
      ${d.items.map(a=>`<div class="card" onclick="editArtist(${a.id},'${(a.name||'').replace(/'/g,"\\'")}','${(a.bio||'').replace(/'/g,"\\'").replace(/\n/g,"\\n")}')">
        <div class="card-artist-wrap">
          <img class="card-img-circle" src="/api/admin/artists/${a.id}/avatar" alt="${a.name}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><circle cx=%22100%22 cy=%22100%22 r=%22100%22 fill=%22%23334155%22/><text fill=%22%2394a3b8%22 font-size=%2264%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.35em%22>🎤</text></svg>'">
          <div class="card-artist-name">${a.name}</div>
        </div>
      </div>`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(artistPage>1){artistPage--;render_artists()}" ${d.page<=1?'disabled':''}>上一页</button>
      <span>第 ${d.page} / ${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){artistPage++;render_artists()}" ${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="artistModal"></div>`;
}
function editArtist(id,name,bio){
  document.getElementById('artistModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑艺人 #${id}</h3>
        <div class="form-group"><label>艺人名</label><input id="eaName" value="${name}"></div>
        <div class="form-group"><label>简介</label><textarea id="eaBio">${bio.replace(/\\n/g,'\n')}</textarea></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveArtist(${id})">保存</button></div>
      </div>
    </div>`;
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
  document.getElementById('content').innerHTML=`
    <div class="header"><h1>👥 用户管理</h1><button class="btn btn-primary" onclick="showAddUser()">+ 添加用户</button></div>
    <table><thead><tr><th>ID</th><th>用户名</th><th>昵称</th><th>角色</th><th>注册时间</th><th>操作</th></tr></thead>
    <tbody>${items.map(u=>`<tr>
      <td>${u.id}</td><td>${u.username}</td><td>${u.displayName||'-'}</td>
      <td><span class="badge badge-${u.role}">${u.role==='admin'?'👑 管理员':'👤 用户'}</span></td>
      <td>${fmtDate(u.createdAt)}</td>
      <td>${u.role!=='admin'? `<button class="btn btn-primary btn-sm" onclick="setAdmin(${u.id})">设为管理员</button>` : ''}
          <button class="btn btn-danger btn-sm" onclick="deleteUser(${u.id})">删除</button></td>
    </tr>`).join('')}</tbody></table>
    <div id="userModal"></div>`;
}
function showAddUser(){
  document.getElementById('userModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>添加用户</h3>
        <div class="form-group"><label>用户名</label><input id="nuUser" placeholder="至少3位"></div>
        <div class="form-group"><label>昵称</label><input id="nuName"></div>
        <div class="form-group"><label>密码</label><input id="nuPass" type="password" placeholder="至少6位"></div>
        <div class="form-group"><label>角色</label><select id="nuRole"><option value="user">普通用户</option><option value="admin">管理员</option></select></div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="addUser()">添加</button></div>
      </div>
    </div>`;
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

// ─── 有声书（卡片网格+排序）──────────────────────────
let abPage=1, abSort='default';
async function render_audiobooks(){
  const search=document.getElementById('abSearch')?.value||'';
  const r=await api('/api/admin/audiobooks?page='+abPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+abSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=`
    <div class="header">
      <h1>📖 有声书 <span style="font-size:14px;color:var(--muted);font-weight:400">共 ${d.total} 本</span></h1>
      <div class="toolbar">
        <input class="search-input" id="abSearch" placeholder="搜索有声书名..." value="${search}" onkeydown="if(event.key==='Enter'){abPage=1;render_audiobooks()}">
        <div class="sort-btns">${sortBtns(abSort,[['default','默认排序'],['recent','最近添加']],'k=>{abSort=k;abPage=1;render_audiobooks()}')}</div>
      </div>
    </div>
    <div class="card-grid">
      ${d.items.map(a=>`<div class="card" onclick="editAudiobook(${a.id},'${(a.title||'').replace(/'/g,"\\'")}','${(a.author||'').replace(/'/g,"\\'")}','${(a.narrator||'').replace(/'/g,"\\'")}','${(a.genre||'').replace(/'/g,"\\'")}',${a.year||0})">
        <img class="card-img" src="/api/cover/${a.id}" alt="${a.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><rect fill=%22%23334155%22 width=%22200%22 height=%22200%22/><text fill=%22%2394a3b8%22 font-size=%2248%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.3em%22>📖</text></svg>'">
        <div class="card-body">
          <div class="card-title">${a.title}</div>
          <div class="card-sub">${a.author||'未知作者'}${a.narrator?' · '+a.narrator:''}</div>
          <div class="card-sub">${a.chapterCount||0}章 · ${fmtDur(a.totalDuration)}</div>
        </div>
      </div>`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(abPage>1){abPage--;render_audiobooks()}" ${d.page<=1?'disabled':''}>上一页</button>
      <span>第 ${d.page} / ${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){abPage++;render_audiobooks()}" ${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="abModal"></div>`;
}
function editAudiobook(id,title,author,narrator,genre,year){
  document.getElementById('abModal').innerHTML=`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑有声书 #${id}</h3>
        <div class="form-group"><label>书名</label><input id="eabTitle" value="${title}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>作者</label><input id="eabAuthor" value="${author}"></div>
          <div class="form-group" style="flex:1"><label>演播者</label><input id="eabNarrator" value="${narrator}"></div>
        </div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>分类</label><input id="eabGenre" value="${genre}"></div>
          <div class="form-group" style="flex:1"><label>年份</label><input id="eabYear" type="number" value="${year}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button>
        <button class="btn btn-primary" onclick="saveAudiobook(${id})">保存</button></div>
      </div>
    </div>`;
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
