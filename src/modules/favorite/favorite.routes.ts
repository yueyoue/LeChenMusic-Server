import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, and, desc } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';

const router = Router();

/** 获取收藏列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const type = req.query.type as 'track' | 'album' | 'artist' | undefined;
    const conditions = [eq(schema.favorite.userId, req.user!.userId)];
    if (type) conditions.push(eq(schema.favorite.targetType, type));

    const favorites = await db.select().from(schema.favorite)
      .where(and(...conditions))
      .orderBy(desc(schema.favorite.createdAt)).all();

    res.json({ code: 0, message: 'ok', data: favorites });
  } catch (err) { next(err); }
});

/** 添加收藏 */
router.post('/', authMiddleware, async (req, res, next) => {
  try {
    const { targetType, targetId } = req.body;
    if (!targetType || !targetId) throw new AppError(2001, 400, 'targetType and targetId required');

    await db.insert(schema.favorite).values({
      userId: req.user!.userId,
      targetType,
      targetId,
    }).onConflictDoNothing();

    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

/** 取消收藏 */
router.delete('/:type/:targetId', authMiddleware, async (req, res, next) => {
  try {
    await db.delete(schema.favorite).where(
      and(
        eq(schema.favorite.userId, req.user!.userId),
        eq(schema.favorite.targetType, req.params.type as 'track' | 'album' | 'artist'),
        eq(schema.favorite.targetId, parseInt(req.params.targetId)),
      )
    );
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

export default router;
