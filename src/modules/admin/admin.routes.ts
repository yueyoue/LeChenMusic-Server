import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, sql, desc, and, or } from 'drizzle-orm';
import { authMiddleware, adminMiddleware } from '../../middleware/auth.js';
import argon2 from 'bcryptjs';
import { AppError } from '../../middleware/error-handler.js';
import { qs, qn } from '../../utils/query.js';

const router = Router();

// 所有 admin 路由都需要管理员权限
router.use(authMiddleware, adminMiddleware);

// ─── 仪表盘 ──────────────────────────────────────────────
router.get('/dashboard', async (req, res, next) => {
  try {
    const [trackCount, albumCount, artistCount, userCount, libraryCount, audiobookCount] = await Promise.all([
      db.select({ count: sql<number>`count(*)` }).from(schema.track).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.album).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.artist).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.sysUser).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.mediaLibrary).get(),
      db.select({ count: sql<number>`count(*)` }).from(schema.audiobook).get(),
    ]);
    const totalSize = await db.select({ sum: sql<number>`coalesce(sum(file_size), 0)` }).from(schema.track).get();

    res.json({
      code: 0, message: 'ok', data: {
        tracks: trackCount?.count ?? 0,
        albums: albumCount?.count ?? 0,
        artists: artistCount?.count ?? 0,
        users: userCount?.count ?? 0,
        libraries: libraryCount?.count ?? 0,
        audiobooks: audiobookCount?.count ?? 0,
        totalSizeBytes: totalSize?.sum ?? 0,
      },
    });
  } catch (err) { next(err); }
});

// ─── 用户管理 ────────────────────────────────────────────
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

router.post('/users', async (req, res, next) => {
  try {
    const { username, password, role, displayName } = req.body;
    if (!username || !password) throw new AppError(2001, 400, 'Username and password required');
    const existing = await db.select().from(schema.sysUser).where(eq(schema.sysUser.username, username)).get();
    if (existing) throw new AppError(2001, 400, 'Username already exists');
    const passwordHash = await argon2.hash(password, 12);
    const user = await db.insert(schema.sysUser).values({
      username, passwordHash, role: role || 'user', displayName: displayName || username,
    }).returning().get();
    res.json({ code: 0, message: 'ok', data: { id: user.id, username: user.username, role: user.role } });
  } catch (err) { next(err); }
});

router.put('/users/:id', async (req, res, next) => {
  try {
    const userId = parseInt(req.params.id as string);
    const { role, displayName, password } = req.body;
    const updates: any = {};
    if (role) updates.role = role;
    if (displayName) updates.displayName = displayName;
    if (password) updates.passwordHash = await argon2.hash(password, 12);
    updates.updatedAt = new Date();
    await db.update(schema.sysUser).set(updates).where(eq(schema.sysUser.id, userId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

router.delete('/users/:id', async (req, res, next) => {
  try {
    const userId = parseInt(req.params.id as string);
    if (userId === req.user!.userId) throw new AppError(4001, 400, 'Cannot delete yourself');
    await db.delete(schema.sysUser).where(eq(schema.sysUser.id, userId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 媒体库管理 ──────────────────────────────────────────
router.get('/libraries', async (req, res, next) => {
  try {
    const libraries = await db.select().from(schema.mediaLibrary).all();
    res.json({ code: 0, message: 'ok', data: libraries });
  } catch (err) { next(err); }
});

router.post('/libraries', async (req, res, next) => {
  try {
    const { name, storageType, storagePath } = req.body;
    if (!name || !storageType || !storagePath) throw new AppError(2001, 400, 'Missing required fields');
    const result = await db.insert(schema.mediaLibrary).values({ name, storageType, storagePath }).returning().get();
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

router.delete('/libraries/:id', async (req, res, next) => {
  try {
    await db.delete(schema.mediaLibrary).where(eq(schema.mediaLibrary.id, parseInt(req.params.id as string)));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

router.post('/libraries/:id/scan', async (req, res, next) => {
  try {
    const libraryId = parseInt(req.params.id as string);
    const { scannerService } = await import('../library/scanner.service.js');
    scannerService.scanLibrary(libraryId).catch(err => console.error('Scan failed:', err));
    res.json({ code: 0, message: 'Scan started', data: { libraryId } });
  } catch (err) { next(err); }
});

// ─── 元数据管理 ──────────────────────────────────────────
// 歌曲列表（含搜索）
router.get('/tracks', async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const where = search ? or(
      like(schema.track.title, `%${search}%`),
      like(schema.track.genre, `%${search}%`),
    ) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select({
        id: schema.track.id,
        title: schema.track.title,
        artistId: schema.track.artistId,
        albumId: schema.track.albumId,
        duration: schema.track.duration,
        bitrate: schema.track.bitrate,
        format: schema.track.format,
        trackNumber: schema.track.trackNumber,
        discNumber: schema.track.discNumber,
        genre: schema.track.genre,
        storagePath: schema.track.storagePath,
        artistName: schema.artist.name,
        albumTitle: schema.album.title,
      })
        .from(schema.track)
        .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
        .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
        .where(where)
        .orderBy(schema.track.id)
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.track).where(where).get(),
    ]);
    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

// 编辑歌曲
router.put('/tracks/:id', async (req, res, next) => {
  try {
    const trackId = parseInt(req.params.id as string);
    const { title, genre, trackNumber, discNumber } = req.body;
    const updates: any = {};
    if (title !== undefined) updates.title = title;
    if (genre !== undefined) updates.genre = genre;
    if (trackNumber !== undefined) updates.trackNumber = trackNumber;
    if (discNumber !== undefined) updates.discNumber = discNumber;
    if (Object.keys(updates).length === 0) throw new AppError(2001, 400, 'No fields to update');
    await db.update(schema.track).set(updates).where(eq(schema.track.id, trackId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// 专辑列表
router.get('/albums', async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const where = search ? like(schema.album.title, `%${search}%`) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select({
        id: schema.album.id,
        title: schema.album.title,
        artistId: schema.album.artistId,
        year: schema.album.year,
        genre: schema.album.genre,
        coverPath: schema.album.coverPath,
        artistName: schema.artist.name,
      })
        .from(schema.album)
        .leftJoin(schema.artist, eq(schema.album.artistId, schema.artist.id))
        .where(where)
        .orderBy(schema.album.id)
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.album).where(where).get(),
    ]);
    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

// 编辑专辑
router.put('/albums/:id', async (req, res, next) => {
  try {
    const albumId = parseInt(req.params.id as string);
    const { title, year, genre, description } = req.body;
    const updates: any = {};
    if (title !== undefined) updates.title = title;
    if (year !== undefined) updates.year = year;
    if (genre !== undefined) updates.genre = genre;
    if (description !== undefined) updates.description = description;
    if (Object.keys(updates).length === 0) throw new AppError(2001, 400, 'No fields to update');
    await db.update(schema.album).set(updates).where(eq(schema.album.id, albumId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// 艺人列表
router.get('/artists', async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const where = search ? like(schema.artist.name, `%${search}%`) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select().from(schema.artist).where(where)
        .orderBy(schema.artist.id)
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.artist).where(where).get(),
    ]);
    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

// 编辑艺人
router.put('/artists/:id', async (req, res, next) => {
  try {
    const artistId = parseInt(req.params.id as string);
    const { name, bio } = req.body;
    const updates: any = {};
    if (name !== undefined) updates.name = name;
    if (bio !== undefined) updates.bio = bio;
    if (Object.keys(updates).length === 0) throw new AppError(2001, 400, 'No fields to update');
    await db.update(schema.artist).set(updates).where(eq(schema.artist.id, artistId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 歌词管理 ────────────────────────────────────────────
router.get('/tracks/:id/lyrics', async (req, res, next) => {
  try {
    const trackId = parseInt(req.params.id as string);
    const lyrics = await db.select().from(schema.lyric).where(eq(schema.lyric.trackId, trackId)).all();
    res.json({ code: 0, message: 'ok', data: lyrics });
  } catch (err) { next(err); }
});

router.post('/tracks/:id/lyrics', async (req, res, next) => {
  try {
    const trackId = parseInt(req.params.id as string);
    const { content, language, type } = req.body;
    if (!content) throw new AppError(2001, 400, 'Content required');
    const result = await db.insert(schema.lyric).values({
      trackId, content, language: language || 'original', type: type || 'lrc',
    }).returning().get();
    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

router.delete('/lyrics/:id', async (req, res, next) => {
  try {
    await db.delete(schema.lyric).where(eq(schema.lyric.id, parseInt(req.params.id as string)));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 系统配置 ────────────────────────────────────────────
router.get('/config', async (req, res, next) => {
  try {
    const configs = await db.select().from(schema.sysConfig).all();
    const map = Object.fromEntries(configs.map(c => [c.key, c.value]));
    res.json({ code: 0, message: 'ok', data: map });
  } catch (err) { next(err); }
});

router.put('/config', async (req, res, next) => {
  try {
    const entries = Object.entries(req.body);
    for (const [key, value] of entries) {
      await db.insert(schema.sysConfig)
        .values({ key, value: String(value), updatedAt: new Date() })
        .onConflictDoUpdate({ target: schema.sysConfig.key, set: { value: String(value), updatedAt: new Date() } });
    }
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 缓存清理 ────────────────────────────────────────────
router.post('/cache/clear', async (req, res, next) => {
  try {
    await db.delete(schema.transcodeCache);
    res.json({ code: 0, message: 'Cache cleared', data: null });
  } catch (err) { next(err); }
});

// ─── 有声书管理 ──────────────────────────────────────────
router.get('/audiobooks', async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const where = search ? like(schema.audiobook.title, `%${search}%`) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select().from(schema.audiobook).where(where)
        .orderBy(desc(schema.audiobook.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.audiobook).where(where).get(),
    ]);
    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

router.put('/audiobooks/:id', async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const { title, author, narrator, genre, year, description } = req.body;
    const updates: any = {};
    if (title !== undefined) updates.title = title;
    if (author !== undefined) updates.author = author;
    if (narrator !== undefined) updates.narrator = narrator;
    if (genre !== undefined) updates.genre = genre;
    if (year !== undefined) updates.year = year;
    if (description !== undefined) updates.description = description;
    if (Object.keys(updates).length === 0) throw new AppError(2001, 400, 'No fields to update');
    await db.update(schema.audiobook).set(updates).where(eq(schema.audiobook.id, bookId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

router.delete('/audiobooks/:id', async (req, res, next) => {
  try {
    await db.delete(schema.audiobook).where(eq(schema.audiobook.id, parseInt(req.params.id as string)));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

router.get('/audiobooks/:id/chapters', async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const chapters = await db.select().from(schema.audiobookChapter)
      .where(eq(schema.audiobookChapter.audiobookId, bookId))
      .orderBy(schema.audiobookChapter.chapterNumber).all();
    res.json({ code: 0, message: 'ok', data: chapters });
  } catch (err) { next(err); }
});

export default router;
