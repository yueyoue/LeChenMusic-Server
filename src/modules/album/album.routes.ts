import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, sql, desc } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';

const router = Router();

/** 获取专辑列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = parseInt(req.query.page as string) || 1;
    const pageSize = parseInt(req.query.pageSize as string) || 50;
    const search = req.query.search as string;
    const artistId = req.query.artistId ? parseInt(req.query.artistId as string) : undefined;

    const conditions = [];
    if (search) conditions.push(like(schema.album.title, `%${search}%`));
    if (artistId) conditions.push(eq(schema.album.artistId, artistId));
    const where = conditions.length > 0 ? conditions.reduce((a, b) => eq(a, b)) : undefined;

    const [items, total] = await Promise.all([
      db.select().from(schema.album).where(where)
        .orderBy(desc(schema.album.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.album).where(where)
        .get().then(r => r?.count ?? 0),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total, page, pageSize } });
  } catch (err) { next(err); }
});

/** 获取专辑详情 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const album = await db.select().from(schema.album)
      .where(eq(schema.album.id, parseInt(req.params.id))).get();
    if (!album) throw new AppError(3001, 404, 'Album not found');
    res.json({ code: 0, message: 'ok', data: album });
  } catch (err) { next(err); }
});

/** 获取专辑下的音轨 */
router.get('/:id/tracks', authMiddleware, async (req, res, next) => {
  try {
    const albumId = parseInt(req.params.id);
    const tracks = await db.select().from(schema.track)
      .where(eq(schema.track.albumId, albumId))
      .orderBy(schema.track.discNumber, schema.track.trackNumber).all();
    res.json({ code: 0, message: 'ok', data: tracks });
  } catch (err) { next(err); }
});

export default router;
