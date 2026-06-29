import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, sql, desc, and } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';

const router = Router();

/** 获取艺人列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = parseInt(req.query.page as string) || 1;
    const pageSize = parseInt(req.query.pageSize as string) || 50;
    const search = req.query.search as string;

    const where = search ? like(schema.artist.name, `%${search}%`) : undefined;

    const [items, total] = await Promise.all([
      db.select().from(schema.artist).where(where)
        .orderBy(desc(schema.artist.createdAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` }).from(schema.artist).where(where)
        .get().then(r => r?.count ?? 0),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total, page, pageSize } });
  } catch (err) { next(err); }
});

/** 获取艺人详情 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const artist = await db.select().from(schema.artist)
      .where(eq(schema.artist.id, parseInt(req.params.id))).get();
    if (!artist) throw new AppError(3001, 404, 'Artist not found');
    res.json({ code: 0, message: 'ok', data: artist });
  } catch (err) { next(err); }
});

/** 获取艺人的专辑 */
router.get('/:id/albums', authMiddleware, async (req, res, next) => {
  try {
    const artistId = parseInt(req.params.id);
    const albums = await db.select().from(schema.album)
      .where(eq(schema.album.artistId, artistId))
      .orderBy(desc(schema.album.year)).all();
    res.json({ code: 0, message: 'ok', data: albums });
  } catch (err) { next(err); }
});

export default router;
