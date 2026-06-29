import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, sql, desc } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';
import { qs, qn } from '../../utils/query.js';

const router = Router();

/** 获取艺人列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;
    const search = qs(req.query.search);

    const where = search ? like(schema.artist.name, `%${search}%`) : undefined;

    const [items, totalResult] = await Promise.all([
      db.select().from(schema.artist).where(where)
        .orderBy(desc(schema.artist.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.artist).where(where).get(),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

/** 获取艺人详情 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const artist = await db.select().from(schema.artist)
      .where(eq(schema.artist.id, parseInt(req.params.id as string))).get();
    if (!artist) throw new AppError(3001, 404, 'Artist not found');
    res.json({ code: 0, message: 'ok', data: artist });
  } catch (err) { next(err); }
});

/** 获取艺人的专辑 */
router.get('/:id/albums', authMiddleware, async (req, res, next) => {
  try {
    const artistId = parseInt(req.params.id as string);
    const albums = await db.select().from(schema.album)
      .where(eq(schema.album.artistId, artistId))
      .orderBy(desc(schema.album.year)).all();
    res.json({ code: 0, message: 'ok', data: albums });
  } catch (err) { next(err); }
});

export default router;
