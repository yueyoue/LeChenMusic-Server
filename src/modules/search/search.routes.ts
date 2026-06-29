import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { like, or, sql } from 'drizzle-orm';
import { authMiddleware } from '../../middleware/auth.js';

const router = Router();

/** 全局搜索 */
router.get('/', authMiddleware, async (req, res, next) => {
  try {
    const q = req.query.q as string;
    if (!q || q.trim().length === 0) {
      return res.json({ code: 0, message: 'ok', data: { tracks: [], albums: [], artists: [] } });
    }

    const pattern = `%${q}%`;
    const limit = 20;

    const [tracks, albums, artists] = await Promise.all([
      db.select({
        id: schema.track.id,
        title: schema.track.title,
        artistId: schema.track.artistId,
        albumId: schema.track.albumId,
        duration: schema.track.duration,
        format: schema.track.format,
      })
        .from(schema.track)
        .where(or(like(schema.track.title, pattern), like(schema.track.genre, pattern)))
        .limit(limit).all(),

      db.select({
        id: schema.album.id,
        title: schema.album.title,
        artistId: schema.album.artistId,
        year: schema.album.year,
        coverPath: schema.album.coverPath,
      })
        .from(schema.album)
        .where(like(schema.album.title, pattern))
        .limit(limit).all(),

      db.select({
        id: schema.artist.id,
        name: schema.artist.name,
        avatarPath: schema.artist.avatarPath,
      })
        .from(schema.artist)
        .where(like(schema.artist.name, pattern))
        .limit(limit).all(),
    ]);

    res.json({ code: 0, message: 'ok', data: { tracks, albums, artists } });
  } catch (err) { next(err); }
});

export default router;
