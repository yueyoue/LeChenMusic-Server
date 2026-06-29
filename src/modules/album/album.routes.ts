import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, sql, desc, and } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';
import { qs, qn } from '../../utils/query.js';

const router = Router();

/** 获取专辑列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);
    const artistId = qn(req.query.artistId);

    const conditions = [];
    if (search) conditions.push(like(schema.album.title, `%${search}%`));
    if (artistId) conditions.push(eq(schema.album.artistId, artistId));
    const where = conditions.length > 0 ? and(...conditions) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select().from(schema.album).where(where)
        .orderBy(desc(schema.album.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.album).where(where).get(),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

/** 获取专辑详情 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const album = await db.select().from(schema.album)
      .where(eq(schema.album.id, parseInt(req.params.id as string))).get();
    if (!album) throw new AppError(3001, 404, 'Album not found');
    res.json({ code: 0, message: 'ok', data: album });
  } catch (err) { next(err); }
});

/** 获取专辑下的音轨 */
router.get('/:id/tracks', authMiddleware, async (req, res, next) => {
  try {
    const albumId = parseInt(req.params.id as string);
    const tracks = await db.select().from(schema.track)
      .where(eq(schema.track.albumId, albumId))
      .orderBy(schema.track.discNumber, schema.track.trackNumber).all();
    res.json({ code: 0, message: 'ok', data: tracks });
  } catch (err) { next(err); }
});

export default router;
