import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, sql, desc } from 'drizzle-orm';
import { authMiddleware, adminMiddleware } from '../../middleware/auth.js';
import bcrypt from 'bcryptjs';
import { AppError } from '../../middleware/error-handler.js';
import { config } from '../../config/index.js';
import { readdirSync, statSync, rmSync, existsSync } from 'fs';
import { join } from 'path';

const router = Router();

// 所有 admin 路由都需要管理员权限
router.use(authMiddleware, adminMiddleware);

/** 仪表盘统计数据 */
router.get('/dashboard', async (req, res, next) => {
  try {
    const [trackCount, albumCount, artistCount, userCount, libraryCount] = await Promise.all([
      db.select({ count: sql<number>`count(*)` }).from(schema.track).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.album).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.artist).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.sysUser).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.mediaLibrary).get(),
    ]);

    // 计算总文件大小
    const totalSize = await db.select({ sum: sql<number>`coalesce(sum(file_size), 0)` })
      .from(schema.track).get();

    res.json({
      code: 0, message: 'ok', data: {
        tracks: trackCount?.count ?? 0,
        albums: albumCount?.count ?? 0,
        artists: artistCount?.count ?? 0,
        users: userCount?.count ?? 0,
        libraries: libraryCount?.count ?? 0,
        totalSizeBytes: totalSize?.sum ?? 0,
      },
    });
  } catch (err) { next(err); }
});

/** 用户列表 */
router.get('/users', async (req, res, next) => {
  try {
    const users = await db.select({
      id: schema.sysUser.id,
      username: schema.sysUser.username,
      role: schema.sysUser.role,
      displayName: schema.sysUser.displayName,
      avatar: schema.sysUser.avatar,
      createdAt: schema.sysUser.createdAt,
    }).from(schema.sysUser).orderBy(desc(schema.sysUser.createdAt)).all();

    res.json({ code: 0, message: 'ok', data: users });
  } catch (err) { next(err); }
});

/** 创建用户 */
router.post('/users', async (req, res, next) => {
  try {
    const { username, password, role, displayName } = req.body;
    if (!username || !password) throw new AppError(2001, 400, 'Username and password required');

    const existing = await db.select().from(schema.sysUser)
      .where(eq(schema.sysUser.username, username)).get();
    if (existing) throw new AppError(2001, 400, 'Username already exists');

    const passwordHash = await bcrypt.hash(password, 12);
    const user = await db.insert(schema.sysUser).values({
      username,
      passwordHash,
      role: role || 'user',
      displayName: displayName || username,
    }).returning().get();

    res.json({ code: 0, message: 'ok', data: { id: user.id, username: user.username, role: user.role } });
  } catch (err) { next(err); }
});

/** 删除用户 */
router.delete('/users/:id', async (req, res, next) => {
  try {
    const userId = parseInt(req.params.id as string);
    if (userId === req.user!.userId) throw new AppError(4001, 400, 'Cannot delete yourself');

    await db.delete(schema.sysUser).where(eq(schema.sysUser.id, userId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

/** 系统配置 */
router.get('/config', async (req, res, next) => {
  try {
    const configs = await db.select().from(schema.sysConfig).all();
    const map = Object.fromEntries(configs.map(c => [c.key, c.value]));
    res.json({ code: 0, message: 'ok', data: map });
  } catch (err) { next(err); }
});

/** 更新系统配置 */
router.put('/config', async (req, res, next) => {
  try {
    const entries = Object.entries(req.body);
    for (const [key, value] of entries) {
      await db.insert(schema.sysConfig)
        .values({ key, value: String(value), updatedAt: new Date() })
        .onConflictDoUpdate({
          target: schema.sysConfig.key,
          set: { value: String(value), updatedAt: new Date() },
        });
    }
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

/** 清理转码缓存 */
router.post('/cache/clear', async (req, res, next) => {
  try {
    const cacheDir = config.transcode.cacheDir;
    if (existsSync(cacheDir)) {
      rmSync(cacheDir, { recursive: true, force: true });
    }
    await db.delete(schema.transcodeCache);
    res.json({ code: 0, message: 'Cache cleared', data: null });
  } catch (err) { next(err); }
});

export default router;
