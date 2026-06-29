import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { existsSync, readFileSync, statSync } from 'fs';
import { join } from 'path';

const router = Router();

/** 获取专辑封面 */
router.get('/:albumId', async (req, res) => {
  try {
    const albumId = parseInt(req.params.albumId as string);
    const album = await db.select().from(schema.album).where(eq(schema.album.id, albumId)).get();

    if (!album?.coverPath) {
      // 返回默认封面
      res.setHeader('Content-Type', 'image/svg+xml');
      res.send('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">🎵</text></svg>');
      return;
    }

    // 查找媒体库路径
    const library = await db.select().from(schema.mediaLibrary).limit(1).get();
    if (!library) { res.status(404).end(); return; }

    const fullPath = join(library.storagePath, album.coverPath);

    if (!existsSync(fullPath)) {
      res.setHeader('Content-Type', 'image/svg+xml');
      res.send('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">🎵</text></svg>');
      return;
    }

    const stat = statSync(fullPath);
    res.setHeader('Content-Type', 'image/jpeg');
    res.setHeader('Content-Length', stat.size);
    res.setHeader('Cache-Control', 'public, max-age=86400');
    res.send(readFileSync(fullPath));
  } catch {
    res.status(500).end();
  }
});

export default router;
