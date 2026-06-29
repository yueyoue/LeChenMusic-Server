import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, and, desc, sql, like } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';
import { qs, qn } from '../../utils/query.js';
import { createReadStream, statSync } from 'fs';
import { join, extname } from 'path';

const router = Router();

// ─── 有声书列表 ──────────────────────────────────────────
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const genre = qs(req.query.genre);

    const conditions = [];
    if (search) conditions.push(like(schema.audiobook.title, `%${search}%`));
    if (genre) conditions.push(eq(schema.audiobook.genre, genre));
    const where = conditions.length > 0 ? and(...conditions) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select().from(schema.audiobook).where(where)
        .orderBy(desc(schema.audiobook.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.audiobook).where(where).get(),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

// ─── 有声书详情 ──────────────────────────────────────────
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const book = await db.select().from(schema.audiobook)
      .where(eq(schema.audiobook.id, bookId)).get();
    if (!book) throw new AppError(3001, 404, 'Audiobook not found');

    const chapters = await db.select().from(schema.audiobookChapter)
      .where(eq(schema.audiobookChapter.audiobookId, bookId))
      .orderBy(schema.audiobookChapter.chapterNumber).all();

    // 获取播放进度
    const progress = await db.select().from(schema.audiobookProgress)
      .where(and(
        eq(schema.audiobookProgress.userId, req.user!.userId),
        eq(schema.audiobookProgress.audiobookId, bookId),
      )).get();

    res.json({ code: 0, message: 'ok', data: { ...book, chapters, progress } });
  } catch (err) { next(err); }
});

// ─── 播放章节 ────────────────────────────────────────────
router.get('/:id/chapters/:chapterId/stream', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const chapterId = parseInt(req.params.chapterId as string);

    const book = await db.select().from(schema.audiobook)
      .where(eq(schema.audiobook.id, bookId)).get();
    if (!book) throw new AppError(3001, 404, 'Audiobook not found');

    const chapter = await db.select().from(schema.audiobookChapter)
      .where(eq(schema.audiobookChapter.id, chapterId)).get();
    if (!chapter) throw new AppError(3001, 404, 'Chapter not found');

    // 查找媒体库路径
    const library = await db.select().from(schema.mediaLibrary)
      .where(eq(schema.mediaLibrary.storageType, book.storageType)).get();
    if (!library) throw new AppError(3002, 404, 'Storage not found');

    const fullPath = join(library.storagePath, chapter.storagePath);
    const stat = statSync(fullPath);
    const ext = extname(fullPath).toLowerCase();

    const mimeMap: Record<string, string> = {
      '.mp3': 'audio/mpeg', '.flac': 'audio/flac', '.aac': 'audio/aac',
      '.ogg': 'audio/ogg', '.wav': 'audio/wav', '.m4a': 'audio/mp4',
      '.opus': 'audio/opus',
    };
    const contentType = mimeMap[ext] || 'application/octet-stream';

    const range = req.headers.range;
    if (range) {
      const parts = range.replace(/bytes=/, '').split('-');
      const start = parseInt(parts[0], 10);
      const end = parts[1] ? parseInt(parts[1], 10) : stat.size - 1;
      res.status(206);
      res.setHeader('Content-Range', `bytes ${start}-${end}/${stat.size}`);
      res.setHeader('Accept-Ranges', 'bytes');
      res.setHeader('Content-Length', end - start + 1);
      res.setHeader('Content-Type', contentType);
      createReadStream(fullPath, { start, end }).pipe(res);
    } else {
      res.setHeader('Content-Length', stat.size);
      res.setHeader('Content-Type', contentType);
      res.setHeader('Accept-Ranges', 'bytes');
      createReadStream(fullPath).pipe(res);
    }
  } catch (err) { next(err); }
});

// ─── 播放进度上报 ────────────────────────────────────────
router.post('/:id/progress', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const { chapterId, chapterNumber, position } = req.body;

    const existing = await db.select().from(schema.audiobookProgress)
      .where(and(
        eq(schema.audiobookProgress.userId, req.user!.userId),
        eq(schema.audiobookProgress.audiobookId, bookId),
      )).get();

    if (existing) {
      await db.update(schema.audiobookProgress)
        .set({ chapterId, chapterNumber, position, lastPlayedAt: new Date() })
        .where(eq(schema.audiobookProgress.id, existing.id));
    } else {
      await db.insert(schema.audiobookProgress).values({
        userId: req.user!.userId,
        audiobookId: bookId,
        chapterId,
        chapterNumber,
        position,
      });
    }

    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 获取播放进度（自动续播）─────────────────────────────
router.get('/:id/progress', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const progress = await db.select().from(schema.audiobookProgress)
      .where(and(
        eq(schema.audiobookProgress.userId, req.user!.userId),
        eq(schema.audiobookProgress.audiobookId, bookId),
      )).get();

    res.json({ code: 0, message: 'ok', data: progress || null });
  } catch (err) { next(err); }
});

// ─── 书签管理 ────────────────────────────────────────────
router.get('/:id/bookmarks', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const bookmarks = await db.select().from(schema.audiobookBookmark)
      .where(and(
        eq(schema.audiobookBookmark.userId, req.user!.userId),
        eq(schema.audiobookBookmark.audiobookId, bookId),
      )).orderBy(desc(schema.audiobookBookmark.createdAt)).all();

    res.json({ code: 0, message: 'ok', data: bookmarks });
  } catch (err) { next(err); }
});

router.post('/:id/bookmarks', authMiddleware, async (req, res, next) => {
  try {
    const bookId = parseInt(req.params.id as string);
    const { chapterId, position, title } = req.body;

    const result = await db.insert(schema.audiobookBookmark).values({
      userId: req.user!.userId,
      audiobookId: bookId,
      chapterId,
      position,
      title: title || `书签 ${new Date().toLocaleString('zh-CN')}`,
    }).returning().get();

    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

router.delete('/bookmarks/:bookmarkId', authMiddleware, async (req, res, next) => {
  try {
    await db.delete(schema.audiobookBookmark)
      .where(eq(schema.audiobookBookmark.id, parseInt(req.params.bookmarkId as string)));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

// ─── 收藏有声书 ──────────────────────────────────────────
router.post('/:id/favorite', authMiddleware, async (req, res, next) => {
  try {
    await db.insert(schema.favorite).values({
      userId: req.user!.userId,
      targetType: 'audiobook',
      targetId: parseInt(req.params.id as string),
    }).onConflictDoNothing();
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

router.delete('/:id/favorite', authMiddleware, async (req, res, next) => {
  try {
    await db.delete(schema.favorite).where(
      and(
        eq(schema.favorite.userId, req.user!.userId),
        eq(schema.favorite.targetType, 'audiobook'),
        eq(schema.favorite.targetId, parseInt(req.params.id as string)),
      )
    );
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

export default router;
