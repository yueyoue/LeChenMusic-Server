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
app.use(helmet());
app.use(cors({
  origin: config.cors.origins.length > 0 ? config.cors.origins : '*',
  credentials: true,
}));
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// 请求频率限制
app.use('/api/auth/login', rateLimit({
  windowMs: 15 * 60 * 1000, // 15 分钟
  max: 20, // 最多 20 次
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

// 首页
app.get('/', (_req, res) => {
  res.send(`<!DOCTYPE html>
<html lang="zh">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width"><title>LeChenMusic Server</title>
<style>*{margin:0;padding:0;box-sizing:border-box}body{font-family:-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;justify-content:center;align-items:center;min-height:100vh}.card{background:#1e293b;border-radius:16px;padding:48px;text-align:center;max-width:480px}h1{font-size:28px;margin-bottom:12px}p{color:#94a3b8;margin-bottom:24px}.btn{display:inline-block;background:#3b82f6;color:#fff;padding:12px 32px;border-radius:8px;text-decoration:none;font-size:16px}.btn:hover{background:#2563eb}.status{margin-top:24px;color:#4ade80;font-size:14px}</style></head>
<body><div class="card">
<h1>🎵 LeChenMusic</h1>
<p>私有音乐流媒体服务端</p>
<a href="/api/health" class="btn">检查服务状态</a>
<div class="status">✅ 服务运行中</div>
</div></body></html>`);
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
