import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, desc, sql } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { qn } from '../../utils/query.js';

const router = Router();

/** 获取播放历史 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const page = qn(req.query.page, 1)!;
    const pageSize = qn(req.query.pageSize, 50)!;

    const [items, totalResult] = await Promise.all([
      db.select({
        id: schema.playHistory.id,
        trackId: schema.playHistory.trackId,
        position: schema.playHistory.position,
        duration: schema.playHistory.duration,
        playedAt: schema.playHistory.playedAt,
        trackTitle: schema.track.title,
        trackFormat: schema.track.format,
        trackDuration: schema.track.duration,
      })
        .from(schema.playHistory)
        .innerJoin(schema.track, eq(schema.playHistory.trackId, schema.track.id))
        .where(eq(schema.playHistory.userId, req.user!.userId))
        .orderBy(desc(schema.playHistory.playedAt))
        .limit(pageSize).offset((page - 1) * pageSize).all(),
      db.select({ count: sql<number>`count(*)` })
        .from(schema.playHistory)
        .where(eq(schema.playHistory.userId, req.user!.userId))
        .get(),
    ]);

    res.json({ code: 0, message: 'ok', data: { items, total: totalResult?.count ?? 0, page, pageSize } });
  } catch (err) { next(err); }
});

/** 上报播放进度 */
router.post('/', authMiddleware, async (req, res, next) => {
  try {
    const { trackId, position, duration } = req.body;
    if (!trackId) return res.status(400).json({ code: 2001, message: 'trackId required' });

    await db.insert(schema.playHistory).values({
      userId: req.user!.userId,
      trackId,
      position: position || 0,
      duration,
    });

    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

export default router;
