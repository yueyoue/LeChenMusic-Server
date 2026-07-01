export const adminPageHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>LeChenMusic 管理后台</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{--bg:#0f172a;--card:#1e293b;--border:#334155;--text:#e2e8f0;--muted:#94a3b8;--primary:#3b82f6;--danger:#ef4444;--success:#22c55e;--green:#22c55e;--brown:#a16207}
body{font-family:-apple-system,system-ui,sans-serif;background:var(--bg);color:var(--text);display:flex;min-height:100vh}
.sidebar{width:220px;background:var(--card);border-right:1px solid var(--border);padding:16px 0;flex-shrink:0;display:flex;flex-direction:column}
.sidebar h2{padding:0 16px 16px;font-size:18px;border-bottom:1px solid var(--border);margin-bottom:8px}
.sidebar .nav-item{display:block;padding:10px 16px;color:var(--muted);text-decoration:none;font-size:14px;cursor:pointer;transition:.15s}
.sidebar .nav-item:hover,.sidebar .nav-item.active{color:var(--text);background:rgba(59,130,246,.1);border-right:3px solid var(--primary)}
.main{flex:1;padding:24px;overflow-y:auto;min-width:0}
.header{display:flex;justify-content:space-between;align-items:center;margin-bottom:20px;flex-wrap:wrap;gap:12px}
.header h1{font-size:22px;white-space:nowrap}
.toolbar{display:flex;gap:10px;align-items:center;flex-wrap:wrap}
.search-input{background:var(--card);border:1px solid var(--border);color:var(--text);padding:8px 14px;border-radius:8px;font-size:13px;outline:none;width:260px}
.search-input:focus{border-color:var(--primary)}
.sort-btns{display:flex;gap:0;background:var(--card);border-radius:8px;overflow:hidden;border:1px solid var(--border)}
.sort-btn{padding:7px 14px;font-size:12px;color:var(--muted);cursor:pointer;border:none;background:transparent;transition:.15s}
.sort-btn.active{background:var(--primary);color:#fff}
.sort-btn:hover:not(.active){color:var(--text);background:rgba(255,255,255,.05)}
.btn{padding:8px 16px;border:none;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500}
.btn-primary{background:var(--primary);color:#fff}
.btn-danger{background:var(--danger);color:#fff}
.btn-success{background:var(--green);color:#fff}
.btn-sm{padding:4px 10px;font-size:12px}
.btn:hover{opacity:.85}
table{width:100%;border-collapse:collapse;background:var(--card);border-radius:8px;overflow:hidden}
th,td{padding:10px 14px;text-align:left;border-bottom:1px solid var(--border);font-size:13px}
th{background:#0f172a;color:var(--muted);font-weight:500;font-size:12px;text-transform:uppercase}
tr:hover{background:rgba(255,255,255,.02)}
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
.pager{display:flex;gap:8px;justify-content:center;margin-top:20px;align-items:center}
.pager button{padding:6px 14px}
.pager span{font-size:13px;color:var(--muted)}
.card-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:20px}
.card{background:var(--card);border-radius:10px;overflow:hidden;cursor:pointer;transition:transform .15s,box-shadow .15s}
.card:hover{transform:translateY(-2px);box-shadow:0 6px 20px rgba(0,0,0,.3)}
.card-img{width:100%;aspect-ratio:1;object-fit:cover;display:block;background:#334155}
.card-img-circle{width:120px;height:120px;border-radius:50%;object-fit:cover;display:block;margin:20px auto 0;background:#334155}
.card-body{padding:10px 12px 14px}
.card-title{font-size:13px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.card-sub{font-size:11px;color:var(--muted);margin-top:3px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.card-artist-wrap{text-align:center;padding:10px 8px 16px}
.card-artist-name{font-size:13px;font-weight:500;margin-top:8px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
/* 媒体库专用样式 */
.lib-entry{background:var(--card);border-radius:10px;padding:16px 20px;margin-bottom:12px;display:flex;align-items:center;gap:20px;border:1px solid var(--border)}
.lib-info{flex:1;min-width:0}
.lib-name{font-size:15px;font-weight:600;display:flex;align-items:center;gap:8px}
.lib-path{font-size:12px;color:var(--muted);margin-top:4px;word-break:break-all}
.lib-meta{display:flex;gap:16px;font-size:12px;color:var(--muted);margin-top:6px;align-items:center}
.lib-meta select{padding:4px 8px;font-size:11px;border-radius:4px}
.lib-actions{display:flex;gap:8px;align-items:center;flex-shrink:0}
.lib-actions .btn{border:1px solid var(--border);background:transparent;color:var(--text);font-size:12px;padding:5px 12px}
.lib-actions .btn:hover{background:rgba(255,255,255,.05)}
.lib-actions .btn-remove{border-color:var(--danger);color:var(--danger)}
.lib-actions .btn-remove:hover{background:rgba(239,68,68,.1)}
.tag{display:inline-block;padding:2px 8px;border-radius:4px;font-size:11px;font-weight:500}
.tag-music{background:#064e3b;color:#6ee7b7}
.tag-audiobook{background:#451a03;color:#fbbf24}
/* 进度条 */
.progress-bar{height:6px;background:#334155;border-radius:3px;overflow:hidden;flex:1}
.progress-fill{height:100%;background:var(--green);border-radius:3px;transition:width .3s}
.scan-status{background:#1a1a2e;border-radius:8px;padding:12px 16px;margin-bottom:16px;display:flex;align-items:center;gap:12px;font-size:13px}
/* 开关 */
.toggle{position:relative;width:36px;height:20px;cursor:pointer}
.toggle input{display:none}
.toggle .slider{position:absolute;top:0;left:0;right:0;bottom:0;background:#334155;border-radius:10px;transition:.2s}
.toggle input:checked+.slider{background:var(--green)}
.toggle .slider:before{content:'';position:absolute;height:16px;width:16px;left:2px;bottom:2px;background:#fff;border-radius:50%;transition:.2s}
.toggle input:checked+.slider:before{transform:translateX(16px)}
/* 目录浏览器 */
.browse-left{width:240px;flex-shrink:0;border-right:1px solid var(--border);overflow-y:auto;max-height:360px}
.browse-left-item{padding:10px 12px;cursor:pointer;font-size:13px;border-bottom:1px solid var(--border);transition:background .1s}
.browse-left-item:hover,.browse-left-item.active{background:rgba(34,197,94,.1)}
.browse-right{flex:1;overflow-y:auto;max-height:360px}
.browse-right-item{padding:8px 12px;cursor:pointer;font-size:13px;display:flex;align-items:center;gap:8px;transition:background .1s}
.browse-right-item:hover{background:rgba(59,130,246,.1)}
@media(max-width:768px){.sidebar{width:60px}.sidebar h2,.sidebar .nav-item span{display:none}.sidebar .nav-item{text-align:center;padding:12px 0}.search-input{width:160px}.card-grid{grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:12px}.lib-entry{flex-direction:column;align-items:flex-start}.lib-actions{width:100%;justify-content:flex-end}}
</style>
</head>
<body>
<div class="sidebar">
  <h2>🎵 管理后台</h2>
  <div class="nav-item" onclick="nav('dashboard')" id="nav-dashboard"><span>📊 仪表盘</span></div>
  <div class="nav-item" onclick="nav('libraries')" id="nav-libraries"><span>📁 媒体库</span></div>
  <div class="nav-item" onclick="nav('tracks')" id="nav-tracks"><span>🎵 歌曲</span></div>
  <div class="nav-item" onclick="nav('albums')" id="nav-albums"><span>💿 专辑</span></div>
  <div class="nav-item" onclick="nav('artists')" id="nav-artists"><span>🎤 艺人</span></div>
  <div class="nav-item" onclick="nav('audiobooks')" id="nav-audiobooks"><span>📖 有声书</span></div>
  <div class="nav-item" onclick="nav('users')" id="nav-users"><span>👥 用户</span></div>
  <a href="/" class="nav-item" style="margin-top:auto"><span>🏠 返回首页</span></a>
</div>
<div class="main">
  <div id="msg" class="msg"></div>
  <div id="content"></div>
</div>

<script>
const API = '';
let token = localStorage.getItem('token');
if (!token) location.href = '/';

function headers() { return { 'Content-Type': 'application/json', 'Authorization': '***' + token }; }
function showMsg(t,ok){const m=document.getElementById('msg');m.textContent=t;m.className='msg show '+(ok?'ok':'err');setTimeout(()=>m.className='msg',3000)}
function fmtDate(d){if(!d)return'-';const dt=new Date(d);if(isNaN(dt))return'-';return dt.toLocaleString('zh-CN')}
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
  document.getElementById('content').innerHTML=\`
    <div class="header"><h1>📊 仪表盘</h1></div>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px">
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${d.tracks}</div><div style="color:var(--muted);font-size:13px">🎵 歌曲</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${d.albums}</div><div style="color:var(--muted);font-size:13px">💿 专辑</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${d.artists}</div><div style="color:var(--muted);font-size:13px">🎤 艺人</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${d.users}</div><div style="color:var(--muted);font-size:13px">👥 用户</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${d.libraries}</div><div style="color:var(--muted);font-size:13px">📁 媒体库</div></div>
      <div style="background:var(--card);padding:20px;border-radius:8px;text-align:center"><div style="font-size:28px;font-weight:700">\${fmtSize(d.totalSizeBytes)}</div><div style="color:var(--muted);font-size:13px">💾 总大小</div></div>
    </div>\`;
}

// ─── 媒体库 ──────────────────────────────────────────
let browsePath = '/';
let authorizedDirs = [];

async function render_libraries(){
  const r=await api('/api/admin/libraries');
  if(r.code!==0)return;
  const items=r.data||[];

  // 统计扫描状态
  let scanningCount = items.filter(l=>l.scanStatus==='scanning').length;
  let totalFiles = items.reduce((s,l)=>s+(l.fileCount||0),0);

  let html = \`
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:20px">
      <h1 style="font-size:22px">多媒体路径与歌曲入库扫描</h1>
      <div style="display:flex;gap:10px">
        <button class="btn btn-success" onclick="showAddLibrary()">+ 添加媒体</button>
        <button class="btn" style="background:#334155;color:var(--text)" onclick="pauseAllScan()">⏸ 暂停扫描</button>
      </div>
    </div>\`;

  // 扫描进度条
  if(scanningCount > 0){
    html += \`
    <div class="scan-status">
      <span style="color:var(--muted)">扫描中 (\${scanningCount} 个媒体库)</span>
      <div class="progress-bar"><div class="progress-fill" style="width:50%"></div></div>
      <span style="color:var(--muted)">进行中...</span>
    </div>\`;
  } else if(items.length > 0){
    html += \`
    <div class="scan-status">
      <span style="color:var(--muted)">空闲 (\${items.length} 个媒体库, 共 \${totalFiles} 个文件)</span>
      <div class="progress-bar"><div class="progress-fill" style="width:100%"></div></div>
      <span style="color:var(--muted)">100%</span>
    </div>\`;
  }

  if(items.length === 0){
    html += \`
    <div style="text-align:center;padding:60px;color:var(--muted)">
      <div style="font-size:48px;margin-bottom:12px">📁</div>
      <div>暂无媒体库，点击「添加媒体」开始</div>
    </div>\`;
  } else {
    for(const l of items){
      const isMusic = l.mediaType !== 'audiobook';
      const tagClass = isMusic ? 'tag-music' : 'tag-audiobook';
      const tagLabel = isMusic ? '音乐媒体' : '有声书';
      const statusLabel = l.scanStatus==='scanning'?'扫描中':l.scanStatus==='error'?'错误':'不监控';
      html += \`
      <div class="lib-entry">
        <div class="lib-info">
          <div class="lib-name">\${l.name} <span class="tag \${tagClass}">\${tagLabel}</span></div>
          <div class="lib-path">\${l.storagePath}</div>
          <div class="lib-meta">
            <span>定时入库</span>
            <select style="padding:3px 6px;font-size:11px;border-radius:4px;background:#334155;border:1px solid var(--border);color:var(--text)">
              <option \${l.scanStatus==='idle'?'selected':''}>不监控</option>
              <option>每次启动</option>
              <option>每小时</option>
              <option>每天</option>
            </select>
            <span>上次: \${fmtDate(l.lastScanAt)}</span>
          </div>
        </div>
        <div class="lib-actions">
          <label class="toggle" title="重建"><input type="checkbox"><span class="slider"></span></label>
          <button class="btn" onclick="scanLib(\${l.id})">扫描</button>
          <button class="btn btn-remove" onclick="deleteLib(\${l.id})">移除</button>
        </div>
      </div>\`;
    }
  }

  html += '<div id="libModal"></div>';
  document.getElementById('content').innerHTML = html;
}

async function pauseAllScan(){
  showMsg('扫描已暂停',true);
}

function showAddLibrary(){
  browsePath = '/';
  document.getElementById('libModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal" style="width:680px;max-height:85vh;display:flex;flex-direction:column">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px">
          <h3 style="margin:0">添加媒体路径</h3>
          <button class="btn" style="background:#334155;font-size:12px" onclick="this.closest('.modal-overlay').remove()">关闭</button>
        </div>
        <div style="font-size:12px;color:var(--muted);margin-bottom:16px">上方填写类型与标签，下方直接选择目录后添加。</div>

        <div class="form-group">
          <label>目录类型</label>
          <div style="display:flex;gap:8px;margin-top:4px">
            <button class="btn btn-success dir-type-btn active" onclick="selectDirType(this,'music')" data-type="music">🎵 音乐媒体</button>
            <button class="btn dir-type-btn" style="background:#334155;color:var(--text)" onclick="selectDirType(this,'playlist')" data-type="playlist">📁 歌单目录</button>
            <button class="btn dir-type-btn" style="background:#334155;color:var(--text)" onclick="selectDirType(this,'audiobook')" data-type="audiobook">🎧 有声书</button>
          </div>
        </div>

        <div class="form-group">
          <label>目录路径</label>
          <div style="display:flex;gap:8px">
            <input id="libPath" placeholder="选择下方目录或手动输入" style="flex:1">
            <button class="btn" style="background:#334155;color:var(--text);white-space:nowrap" onclick="useSelectedDir()">使用当前选中</button>
          </div>
        </div>

        <div class="form-group">
          <label>可选标签</label>
          <input id="libTag" placeholder="例如：家庭 NAS / 外接硬盘">
        </div>

        <div class="form-group">
          <label>媒体文件夹</label>
          <div style="font-size:11px;color:var(--muted);margin-bottom:6px">当前为 FPK 模式，仅显示应用授权可读取目录。</div>
          <div style="display:flex;border:1px solid var(--border);border-radius:8px;overflow:hidden;height:300px">
            <div class="browse-left" id="browseLeft"></div>
            <div class="browse-right" id="browseRight">
              <div style="padding:12px;color:var(--muted);text-align:center;font-size:13px">← 请先选择左侧目录</div>
            </div>
          </div>
        </div>

        <div style="font-size:11px;color:var(--muted);margin-top:4px">目录下根目录和子目录的音乐媒体逐一入库，含有过滤重复文件。</div>

        <div class="actions" style="margin-top:16px">
          <button class="btn" style="background:#334155;color:var(--text)" onclick="this.closest('.modal-overlay').remove()">取消</button>
          <button class="btn btn-success" onclick="addLibrary()">添加路径</button>
        </div>
      </div>
    </div>\`;
  loadAuthorizedDirs();
}

let selectedDirType = 'music';
function selectDirType(btn, type){
  selectedDirType = type;
  document.querySelectorAll('.dir-type-btn').forEach(b=>{
    if(b.classList.contains('active')){b.classList.remove('active');b.style.background='#334155';b.style.color='var(--text)'}
  });
  btn.classList.add('active');
  btn.style.background='var(--green)';
  btn.style.color='#fff';
}

let selectedAuthDir = '';
async function loadAuthorizedDirs(){
  const r = await api('/api/admin/authorized-dirs');
  if(r.code!==0) return;
  authorizedDirs = r.data || [];
  const left = document.getElementById('browseLeft');
  if(authorizedDirs.length === 0){
    left.innerHTML = '<div style="padding:20px;color:var(--muted);text-align:center;font-size:12px">未检测到授权目录</div>';
    return;
  }
  left.innerHTML = authorizedDirs.map((d,i)=>\`
    <div class="browse-left-item \${i===0?'active':''}" onclick="selectAuthDir(this,'\${d.path.replace(/'/g,"\\\\'")}')">
      <div style="font-weight:500">授权目录 \${d.name}</div>
      <div style="font-size:11px;color:var(--muted);margin-top:2px">\${d.path}</div>
    </div>\`).join('');
  // 自动选中第一个
  selectAuthDir(left.querySelector('.browse-left-item'), authorizedDirs[0].path);
}

function selectAuthDir(el, path){
  document.querySelectorAll('.browse-left-item').forEach(e=>e.classList.remove('active'));
  if(el) el.classList.add('active');
  selectedAuthDir = path;
  browseDir(path);
}

async function browseDir(path){
  const r=await api('/api/admin/browse?path='+encodeURIComponent(path));
  if(r.code!==0)return;
  const d=r.data;
  browsePath=d.current;
  document.getElementById('libPath').value=d.current;
  const right=document.getElementById('browseRight');
  if(d.entries.length===0){
    right.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted);font-size:13px">此目录下没有子文件夹</div>';
    return;
  }
  let html=\`<div style="padding:6px 12px;border-bottom:1px solid var(--border);display:flex;align-items:center;gap:8px;font-size:12px">
    <button class="btn btn-sm" style="background:#334155;color:var(--text);font-size:11px;padding:3px 8px" onclick="browseUp()">返回上级</button>
    <span style="color:var(--muted)">\${d.current}</span>
  </div>\`;
  for(const e of d.entries){
    const encPath = encodeURIComponent(e.path);
    html += \`<div class="browse-right-item" onclick="browseDir(decodeURIComponent(\'\${encPath}\'))">
      <span>[DIR]</span><span>\${e.name}</span>\
    </div>\`;
  }
  right.innerHTML=html;
}

function browseUp(){
  api('/api/admin/browse?path='+encodeURIComponent(browsePath)).then(r=>{
    if(r.code===0) browseDir(r.data.parent);
  });
}

function useSelectedDir(){
  document.getElementById('libPath').value = browsePath;
}

async function addLibrary(){
  const storagePath=document.getElementById('libPath').value;
  const tag=document.getElementById('libTag').value;
  if(!storagePath)return showMsg('请选择目录路径',false);
  const mediaType = selectedDirType === 'audiobook' ? 'audiobook' : 'music';
  const name = tag || storagePath.split('/').filter(Boolean).pop() || '媒体库';
  const r=await api('/api/admin/libraries',{method:'POST',body:JSON.stringify({name,storageType:'local',storagePath,mediaType})});
  if(r.code===0){showMsg('添加成功',true);document.querySelector('.modal-overlay').remove();render_libraries();}
}

async function scanLib(id){
  const r=await api('/api/admin/libraries/'+id+'/scan',{method:'POST'});
  if(r.code===0)showMsg('扫描已启动，请稍候...',true);
}
async function deleteLib(id){
  if(!confirm('确定移除此媒体库？'))return;
  const r=await api('/api/admin/libraries/'+id,{method:'DELETE'});
  if(r.code===0){showMsg('已移除',true);render_libraries();}
}

// ─── 歌曲（表格+排序）────────────────────────────────
let trackPage=1, trackSort='default';

function setTrackSort(k){trackSort=k;trackPage=1;render_tracks()}
function setAlbumSort(k){albumSort=k;albumPage=1;render_albums()}
function setArtistSort(k){artistSort=k;artistPage=1;render_artists()}
function setAbSort(k){abSort=k;abPage=1;render_audiobooks()}

function sortBtns(current,items,callback){
  return items.map(([k,l])=>\`<button class="sort-btn \${current===k?'active':''}" onclick="\${callback}('\${k}')">\${l}</button>\`).join('');
}
async function render_tracks(){
  const search=document.getElementById('trackSearch')?.value||'';
  const r=await api('/api/admin/tracks?page='+trackPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+trackSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header">
      <h1>🎵 歌曲 <span style="font-size:14px;color:var(--muted);font-weight:400">共 \${d.total} 首</span></h1>
      <div class="toolbar">
        <input class="search-input" id="trackSearch" placeholder="搜索歌曲名、流派..." value="\${search}" onkeydown="if(event.key==='Enter'){trackPage=1;render_tracks()}">
        <div class="sort-btns">\${sortBtns(trackSort,[['default','默认排序'],['recent','最近添加'],['plays','最多播放']],'setTrackSort')}</div>
      </div>
    </div>
    <div class="card-grid">
      \${d.items.map(t=>\`<div class="card" onclick="editTrack(\${t.id},'\${(t.title||'').replace(/'/g,"\\\\'")}','\${(t.genre||'').replace(/'/g,"\\\\'")}',\${t.trackNumber||0},\${t.discNumber||1})">
        <div style="position:relative">
          <img class="card-img" src="/api/cover/track/\${t.id}" alt="\${t.title}" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
          <div style="width:100%;aspect-ratio:1;background:#334155;display:none;align-items:center;justify-content:center;font-size:48px">🎵</div>
        </div>
        <div class="card-body">
          <div class="card-title">\${t.title}</div>
          <div class="card-sub">\${t.artistName||'未知艺人'}\${t.albumTitle?' · '+t.albumTitle:''}</div>
          <div class="card-sub">\${fmtDur(t.duration)}\${t.format?' · '+t.format.toUpperCase():''}\${t.bitrate?' · '+t.bitrate+'kbps':''}</div>
        </div>
      </div>\`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(trackPage>1){trackPage--;render_tracks()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){trackPage++;render_tracks()}" \${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="trackModal"></div>\`;
}
function editTrack(id,title,genre,trackNo,discNo){
  api('/api/admin/tracks/'+id).then(r=>{
    if(r.code!==0)return;
    const t=r.data;
    document.getElementById('trackModal').innerHTML=\`
      <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
        <div class="modal"><h3>编辑歌曲 #\${id}</h3>
          <div class="form-group"><label>歌曲名</label><input id="etTitle" value="\${t.title||''}"></div>
          <div class="form-group"><label>流派</label><input id="etGenre" value="\${t.genre||''}"></div>
          <div style="display:flex;gap:8px">
            <div class="form-group" style="flex:1"><label>曲目号</label><input id="etTrack" type="number" value="\${t.trackNumber||0}"></div>
            <div class="form-group" style="flex:1"><label>碟片号</label><input id="etDisc" type="number" value="\${t.discNumber||1}"></div>
          </div>
          <div style="display:flex;gap:8px">
            <div class="form-group" style="flex:1"><label>格式</label><input value="\${t.format||'-'}" disabled></div>
            <div class="form-group" style="flex:1"><label>码率</label><input value="\${t.bitrate?t.bitrate+'kbps':'-'}" disabled></div>
          </div>
          <div style="display:flex;gap:8px">
            <div class="form-group" style="flex:1"><label>采样率</label><input value="\${t.sampleRate?t.sampleRate+'Hz':'-'}" disabled></div>
            <div class="form-group" style="flex:1"><label>位深</label><input value="\${t.bitDepth?t.bitDepth+'bit':'-'}" disabled></div>
          </div>
          <div class="form-group"><label>文件大小</label><input value="\${fmtSize(t.fileSize)}" disabled></div>
          <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button><button class="btn btn-primary" onclick="saveTrack(\${id})">保存</button></div>
        </div>
      </div>\`;
  });
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
  document.getElementById('content').innerHTML=\`
    <div class="header">
      <h1>💿 专辑 <span style="font-size:14px;color:var(--muted);font-weight:400">共 \${d.total} 张</span></h1>
      <div class="toolbar">
        <input class="search-input" id="albumSearch" placeholder="搜索专辑名..." value="\${search}" onkeydown="if(event.key==='Enter'){albumPage=1;render_albums()}">
        <div class="sort-btns">\${sortBtns(albumSort,[['default','默认排序'],['recent','最近添加'],['plays','最多播放']],'setAlbumSort')}</div>
      </div>
    </div>
    <div class="card-grid">
      \${d.items.map(a=>\`<div class="card" onclick="editAlbum(\${a.id},'\${(a.title||'').replace(/'/g,"\\\\'")}',\${a.year||0},'\${(a.genre||'').replace(/'/g,"\\\\'")}')">
        <img class="card-img" src="/api/cover/\${a.id}" alt="\${a.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><rect fill=%22%23334155%22 width=%22200%22 height=%22200%22/><text fill=%22%2394a3b8%22 font-size=%2248%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.3em%22>💿</text></svg>'">
        <div class="card-body">
          <div class="card-title">\${a.title}</div>
          <div class="card-sub">\${a.artistName||'未知艺人'}\${a.year?' · '+a.year:''}</div>
        </div>
      </div>\`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(albumPage>1){albumPage--;render_albums()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)||1} 页</span>
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

// ─── 艺人（圆形头像卡片+排序）────────────────────────
let artistPage=1, artistSort='default';
async function render_artists(){
  const search=document.getElementById('artistSearch')?.value||'';
  const r=await api('/api/admin/artists?page='+artistPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+artistSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header">
      <h1>🎤 艺人 <span style="font-size:14px;color:var(--muted);font-weight:400">共 \${d.total} 位</span></h1>
      <div class="toolbar">
        <input class="search-input" id="artistSearch" placeholder="搜索艺人名..." value="\${search}" onkeydown="if(event.key==='Enter'){artistPage=1;render_artists()}">
        <div class="sort-btns">\${sortBtns(artistSort,[['default','默认排序'],['recent','最近添加'],['plays','最多播放']],'setArtistSort')}</div>
      </div>
    </div>
    <div class="card-grid">
      \${d.items.map(a=>\`<div class="card" onclick="editArtist(\${a.id},'\${(a.name||'').replace(/'/g,"\\\\'")}','\${(a.bio||'').replace(/'/g,"\\\\'").replace(/\\n/g,"\\\\n")}')">
        <div class="card-artist-wrap">
          <img class="card-img-circle" src="/api/admin/artists/\${a.id}/avatar" alt="\${a.name}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><circle cx=%22100%22 cy=%22100%22 r=%22100%22 fill=%22%23334155%22/><text fill=%22%2394a3b8%22 font-size=%2264%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.35em%22>🎤</text></svg>'">
          <div class="card-artist-name">\${a.name}</div>
        </div>
      </div>\`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(artistPage>1){artistPage--;render_artists()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)||1} 页</span>
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

// ─── 有声书（卡片网格+排序）──────────────────────────
let abPage=1, abSort='default';
async function render_audiobooks(){
  const search=document.getElementById('abSearch')?.value||'';
  const r=await api('/api/admin/audiobooks?page='+abPage+'&pageSize=30&search='+encodeURIComponent(search)+'&sort='+abSort);
  if(r.code!==0)return;
  const d=r.data;
  document.getElementById('content').innerHTML=\`
    <div class="header">
      <h1>📖 有声书 <span style="font-size:14px;color:var(--muted);font-weight:400">共 \${d.total} 本</span></h1>
      <div class="toolbar">
        <input class="search-input" id="abSearch" placeholder="搜索有声书名..." value="\${search}" onkeydown="if(event.key==='Enter'){abPage=1;render_audiobooks()}">
        <div class="sort-btns">\${sortBtns(abSort,[['default','默认排序'],['recent','最近添加']],'setAbSort')}</div>
      </div>
    </div>
    <div class="card-grid">
      \${d.items.map(a=>\`<div class="card" onclick="editAudiobook(\${a.id},'\${(a.title||'').replace(/'/g,"\\\\'")}','\${(a.author||'').replace(/'/g,"\\\\'")}','\${(a.narrator||'').replace(/'/g,"\\\\'")}','\${(a.genre||'').replace(/'/g,"\\\\'")}',\${a.year||0})">
        <img class="card-img" src="/api/cover/\${a.id}" alt="\${a.title}" onerror="this.src='data:image/svg+xml,<svg xmlns=%22http://www.w3.org/2000/svg%22 width=%22200%22 height=%22200%22><rect fill=%22%23334155%22 width=%22200%22 height=%22200%22/><text fill=%22%2394a3b8%22 font-size=%2248%22 x=%2250%%22 y=%2250%%22 text-anchor=%22middle%22 dy=%22.3em%22>📖</text></svg>'">
        <div class="card-body">
          <div class="card-title">\${a.title}</div>
          <div class="card-sub">\${a.author||'未知作者'}\${a.narrator?' · '+a.narrator:''}</div>
          <div class="card-sub">\${a.chapterCount||0}章 · \${fmtDur(a.totalDuration)}</div>
        </div>
      </div>\`).join('')}
    </div>
    <div class="pager">
      <button class="btn btn-sm" onclick="if(abPage>1){abPage--;render_audiobooks()}" \${d.page<=1?'disabled':''}>上一页</button>
      <span>第 \${d.page} / \${Math.ceil(d.total/d.pageSize)||1} 页</span>
      <button class="btn btn-sm" onclick="if(d.page*d.pageSize<d.total){abPage++;render_audiobooks()}" \${d.page*d.pageSize>=d.total?'disabled':''}>下一页</button>
    </div>
    <div id="abModal"></div>\`;
}
function editAudiobook(id,title,author,narrator,genre,year){
  document.getElementById('abModal').innerHTML=\`
    <div class="modal-overlay" onclick="if(event.target===this)this.remove()">
      <div class="modal"><h3>编辑有声书 #\${id}</h3>
        <div class="form-group"><label>书名</label><input id="eabTitle" value="\${title}"></div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>作者</label><input id="eabAuthor" value="\${author}"></div>
          <div class="form-group" style="flex:1"><label>演播者</label><input id="eabNarrator" value="\${narrator}"></div>
        </div>
        <div style="display:flex;gap:8px">
          <div class="form-group" style="flex:1"><label>分类</label><input id="eabGenre" value="\${genre}"></div>
          <div class="form-group" style="flex:1"><label>年份</label><input id="eabYear" type="number" value="\${year}"></div>
        </div>
        <div class="actions"><button class="btn" onclick="this.closest('.modal-overlay').remove()">取消</button>
        <button class="btn btn-primary" onclick="saveAudiobook(\${id})">保存</button></div>
      </div>
    </div>\`;
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
