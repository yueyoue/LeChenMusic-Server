import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, and, sql, desc } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';
import { AppError } from '../../middleware/error-handler.js';

const router = Router();

/** 获取当前用户的歌单列表 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const playlists = await db.select().from(schema.playlist)
      .where(eq(schema.playlist.userId, req.user!.userId))
      .orderBy(desc(schema.playlist.updatedAt)).all();
    res.json({ code: 0, message: 'ok', data: playlists });
  } catch (err) { next(err); }
});

/** 创建歌单 */
router.post('/', authMiddleware, async (req, res, next) => {
  try {
    const { name, description } = req.body;
    if (!name) throw new AppError(2001, 400, 'Name is required');

    const result = await db.insert(schema.playlist).values({
      userId: req.user!.userId,
      name,
      description,
    }).returning().get();

    res.json({ code: 0, message: 'ok', data: result });
  } catch (err) { next(err); }
});

/** 获取歌单详情及歌曲 */
router.get('/:id', authMiddleware, async (req, res, next) => {
  try {
    const playlistId = parseInt(req.params.id);
    const playlist = await db.select().from(schema.playlist)
      .where(eq(schema.playlist.id, playlistId)).get();
    if (!playlist) throw new AppError(3001, 404, 'Playlist not found');

    const tracks = await db.select({
      track: schema.track,
      sortOrder: schema.playlistTrack.sortOrder,
      addedAt: schema.playlistTrack.addedAt,
    })
      .from(schema.playlistTrack)
      .innerJoin(schema.track, eq(schema.playlistTrack.trackId, schema.track.id))
      .where(eq(schema.playlistTrack.playlistId, playlistId))
      .orderBy(schema.playlistTrack.sortOrder).all();

    res.json({ code: 0, message: 'ok', data: { ...playlist, tracks } });
  } catch (err) { next(err); }
});

/** 向歌单添加歌曲 */
router.post('/:id/tracks', authMiddleware, async (req, res, next) => {
  try {
    const playlistId = parseInt(req.params.id);
    const { trackIds } = req.body;
    if (!Array.isArray(trackIds)) throw new AppError(2001, 400, 'trackIds must be an array');

    // 获取当前最大排序
    const maxOrder = await db.select({ max: sql<number>`coalesce(max(sort_order), 0)` })
      .from(schema.playlistTrack)
      .where(eq(schema.playlistTrack.playlistId, playlistId))
      .get();

    let order = (maxOrder?.max ?? 0) + 1;
    for (const trackId of trackIds) {
      await db.insert(schema.playlistTrack).values({
        playlistId,
        trackId,
        sortOrder: order++,
      }).onConflictDoNothing();
    }

    // 更新歌单的 updatedAt
    await db.update(schema.playlist)
      .set({ updatedAt: new Date() })
      .where(eq(schema.playlist.id, playlistId));

    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

/** 从歌单移除歌曲 */
router.delete('/:id/tracks/:trackId', authMiddleware, async (req, res, next) => {
  try {
    const playlistId = parseInt(req.params.id);
    const trackId = parseInt(req.params.trackId);

    await db.delete(schema.playlistTrack).where(
      and(
        eq(schema.playlistTrack.playlistId, playlistId),
        eq(schema.playlistTrack.trackId, trackId),
      )
    );

    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

/** 删除歌单 */
router.delete('/:id', authMiddleware, async (req, res, next) => {
  try {
    const playlistId = parseInt(req.params.id);
    await db.delete(schema.playlist).where(eq(schema.playlist.id, playlistId));
    res.json({ code: 0, message: 'ok', data: null });
  } catch (err) { next(err); }
});

export default router;
