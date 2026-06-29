import express from 'express';
import cors from 'cors';
import helmet from 'helmet';
import rateLimit from 'express-rate-limit';
import { config } from './config/index.js';
import { logger } from './utils/logger.js';
import { errorHandler } from './middleware/error-handler.js';

// 路由
import authRoutes from './modules/auth/auth.routes.js';
import trackRoutes from './modules/track/track.routes.js';
import artistRoutes from './modules/artist/artist.routes.js';
import albumRoutes from './modules/album/album.routes.js';
import searchRoutes from './modules/search/search.routes.js';
import libraryRoutes from './modules/library/library.routes.js';
import playlistRoutes from './modules/playlist/playlist.routes.js';
import favoriteRoutes from './modules/favorite/favorite.routes.js';
import historyRoutes from './modules/history/history.routes.js';
import adminRoutes from './modules/admin/admin.routes.js';

const app = express();

// ─── 中间件 ───────────────────────────────────────────────
app.use(helmet({ contentSecurityPolicy: false }));
app.use(cors({
  origin: config.cors.origins.length > 0 ? config.cors.origins : '*',
  credentials: true,
}));
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// 请求频率限制
app.use('/api/auth/login', rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 20,
  message: { code: 5001, message: 'Too many login attempts, please try again later' },
}));

// 请求日志
app.use((req, res, next) => {
  const start = Date.now();
  res.on('finish', () => {
    logger.info({
      method: req.method,
      url: req.url,
      status: res.statusCode,
      ms: Date.now() - start,
    }, 'request');
  });
  next();
});

// ─── 路由挂载 ─────────────────────────────────────────────
app.use('/api/auth', authRoutes);
app.use('/api/tracks', trackRoutes);
app.use('/api/artists', artistRoutes);
app.use('/api/albums', albumRoutes);
app.use('/api/search', searchRoutes);
app.use('/api/libraries', libraryRoutes);
app.use('/api/playlists', playlistRoutes);
app.use('/api/favorites', favoriteRoutes);
app.use('/api/history', historyRoutes);
app.use('/api/admin', adminRoutes);

// 健康检查
app.get('/api/health', (_req, res) => {
  res.json({ code: 0, message: 'ok', data: { status: 'running', version: '0.1.0' } });
});

// ─── 首页（含注册/登录/管理界面）──────────────────────────
app.get('/', (_req, res) => {
  res.send(`<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>LeChenMusic</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,system-ui,sans-serif;background:#0f172a;color:#e2e8f0;min-height:100vh}
.container{max-width:420px;margin:0 auto;padding:40px 20px}
h1{text-align:center;font-size:28px;margin-bottom:8px}
.subtitle{text-align:center;color:#94a3b8;margin-bottom:32px;font-size:14px}
.tabs{display:flex;gap:0;margin-bottom:24px}
.tab{flex:1;padding:10px;text-align:center;background:#1e293b;color:#94a3b8;cursor:pointer;border:none;font-size:15px;transition:.2s}
.tab:first-child{border-radius:8px 0 0 8px}
.tab:last-child{border-radius:0 8px 8px 0}
.tab.active{background:#3b82f6;color:#fff}
.form{display:flex;flex-direction:column;gap:12px}
input{background:#1e293b;border:1px solid #334155;color:#e2e8f0;padding:12px 16px;border-radius:8px;font-size:15px;outline:none}
input:focus{border-color:#3b82f6}
input::placeholder{color:#64748b}
button.submit{background:#3b82f6;color:#fff;padding:12px;border:none;border-radius:8px;font-size:15px;cursor:pointer;font-weight:600}
button.submit:hover{background:#2563eb}
button.submit:disabled{opacity:.5;cursor:not-allowed}
.msg{padding:12px;border-radius:8px;font-size:14px;display:none;word-break:break-all}
.msg.ok{background:#064e3b;color:#6ee7b7;display:block}
.msg.err{background:#7f1d1d;color:#fca5a5;display:block}
.info{margin-top:24px;padding:16px;background:#1e293b;border-radius:8px;font-size:13px;color:#94a3b8}
.info code{background:#334155;padding:2px 6px;border-radius:4px;color:#e2e8f0}
.token-box{margin-top:16px;padding:12px;background:#064e3b;border-radius:8px;font-size:12px;color:#6ee7b7;word-break:break-all;display:none}
</style>
</head>
<body>
<div class="container">
  <h1>🎵 LeChenMusic</h1>
  <p class="subtitle">私有音乐流媒体服务端</p>

  <div class="tabs">
    <button class="tab active" onclick="switchTab('login')">登录</button>
    <button class="tab" onclick="switchTab('register')">注册</button>
  </div>

  <div id="loginForm" class="form">
    <input id="loginUser" placeholder="用户名" autocomplete="username">
    <input id="loginPass" type="password" placeholder="密码" autocomplete="current-password">
    <button class="submit" onclick="doLogin()">登录</button>
  </div>

  <div id="registerForm" class="form" style="display:none">
    <input id="regUser" placeholder="用户名（至少3位）" autocomplete="username">
    <input id="regName" placeholder="显示名称（可选）">
    <input id="regPass" type="password" placeholder="密码（至少6位）" autocomplete="new-password">
    <button class="submit" onclick="doRegister()">注册</button>
  </div>

  <div id="msg" class="msg"></div>
  <div id="tokenBox" class="token-box"></div>

  <div id="dashboard" style="display:none;margin-top:24px">
    <div class="info" id="userInfo"></div>
    <div class="info" style="margin-top:12px" id="statsInfo">加载中...</div>
    <div style="margin-top:16px;text-align:center">
      <button class="submit" onclick="logout()" style="background:#ef4444;width:100%">退出登录</button>
    </div>
  </div>

  <div class="info" id="apiInfo">
    <b>API 接口</b><br><br>
    <code>POST /api/auth/register</code> 注册<br>
    <code>POST /api/auth/login</code> 登录<br>
    <code>GET /api/tracks</code> 歌曲列表<br>
    <code>GET /api/tracks/:id/stream</code> 播放<br>
    <code>GET /api/search?q=xxx</code> 搜索<br>
    <code>GET /api/admin/dashboard</code> 仪表盘（管理员）<br><br>
    首个注册的用户自动成为管理员
  </div>
</div>

<script>
const API = '';
let token = localStorage.getItem('token');
let userInfo = null;

function switchTab(tab) {
  document.querySelectorAll('.tab').forEach((t,i) => {
    t.classList.toggle('active', (tab==='login'?i===0:i===1));
  });
  document.getElementById('loginForm').style.display = tab==='login'?'flex':'none';
  document.getElementById('registerForm').style.display = tab==='register'?'flex':'none';
  hideMsg();
}

function showMsg(text, ok) {
  const el = document.getElementById('msg');
  el.textContent = text;
  el.className = 'msg ' + (ok ? 'ok' : 'err');
}

function hideMsg() {
  document.getElementById('msg').className = 'msg';
}

async function api(path, opts) {
  const res = await fetch(API + path, {
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) },
    ...opts,
  });
  return res.json();
}

async function doRegister() {
  const username = document.getElementById('regUser').value.trim();
  const displayName = document.getElementById('regName').value.trim();
  const password = document.getElementById('regPass').value;
  if (!username || !password) return showMsg('请填写用户名和密码', false);

  const res = await api('/api/auth/register', {
    method: 'POST',
    body: JSON.stringify({ username, password, displayName: displayName || undefined }),
  });
  if (res.code === 0) {
    showMsg('注册成功！角色: ' + res.data.role + (res.data.role === 'admin' ? '（首个用户，自动管理员）' : '') + '，请登录', true);
    switchTab('login');
    document.getElementById('loginUser').value = username;
  } else {
    showMsg(res.message, false);
  }
}

async function doLogin() {
  const username = document.getElementById('loginUser').value.trim();
  const password = document.getElementById('loginPass').value;
  if (!username || !password) return showMsg('请填写用户名和密码', false);

  const res = await api('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  if (res.code === 0) {
    token = res.data.accessToken;
    userInfo = res.data.user;
    localStorage.setItem('token', token);
    localStorage.setItem('refreshToken', res.data.refreshToken);
    showMsg('登录成功！', true);
    showDashboard();
  } else {
    showMsg(res.message, false);
  }
}

async function showDashboard() {
  document.getElementById('loginForm').style.display = 'none';
  document.getElementById('registerForm').style.display = 'none';
  document.querySelectorAll('.tab').forEach(t => t.style.display = 'none');
  document.getElementById('apiInfo').style.display = 'none';
  document.getElementById('dashboard').style.display = 'block';
  document.getElementById('tokenBox').style.display = 'block';
  document.getElementById('tokenBox').textContent = 'Token: ' + token;

  if (userInfo) {
    document.getElementById('userInfo').innerHTML =
      '<b>当前用户</b><br><br>' +
      '用户名: ' + userInfo.username + '<br>' +
      '角色: ' + (userInfo.role === 'admin' ? '👑 管理员' : '👤 普通用户') + '<br>' +
      '昵称: ' + (userInfo.displayName || '-');
  }

  // 加载统计
  if (userInfo && userInfo.role === 'admin') {
    const stats = await api('/api/admin/dashboard');
    if (stats.code === 0) {
      const d = stats.data;
      document.getElementById('statsInfo').innerHTML =
        '<b>📊 媒体库统计</b><br><br>' +
        '🎵 歌曲: ' + d.tracks + '<br>' +
        '💿 专辑: ' + d.albums + '<br>' +
        '🎤 艺人: ' + d.artists + '<br>' +
        '👥 用户: ' + d.users + '<br>' +
        '📁 媒体库: ' + d.libraries + '<br>' +
        '💾 总大小: ' + formatSize(d.totalSizeBytes);
    }
  } else {
    document.getElementById('statsInfo').innerHTML = '<b>普通用户</b><br><br>联系管理员获取更多权限';
  }
}

function formatSize(bytes) {
  if (bytes === 0) return '0 B';
  const k = 1024, sizes = ['B','KB','MB','GB','TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i];
}

function logout() {
  token = null;
  userInfo = null;
  localStorage.removeItem('token');
  localStorage.removeItem('refreshToken');
  document.getElementById('dashboard').style.display = 'none';
  document.getElementById('tokenBox').style.display = 'none';
  document.querySelectorAll('.tab').forEach(t => t.style.display = '');
  document.getElementById('apiInfo').style.display = '';
  switchTab('login');
  hideMsg();
}

// 自动登录
if (token) {
  api('/api/admin/dashboard').then(res => {
    if (res.code === 0) {
      userInfo = { username: 'admin', role: 'admin', displayName: 'Admin' };
      showDashboard();
    } else {
      token = null;
      localStorage.removeItem('token');
    }
  });
}
</script>
</body></html>`);
});

// 404
app.use((_req, res) => {
  res.status(404).json({ code: 3001, message: 'Not found', data: null });
});

// 错误处理
app.use(errorHandler);

// ─── 启动服务器 ───────────────────────────────────────────
app.listen(config.port, config.host, () => {
  logger.info(`🎵 LeChenMusic Server v0.1.0`);
  logger.info(`📡 Listening on http://${config.host}:${config.port}`);
  logger.info(`📂 Database: ${config.db.path}`);
  logger.info(`🔊 Max transcode tasks: ${config.transcode.maxTasks}`);
});
