import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { existsSync, readFileSync, statSync, readdirSync } from 'fs';
import { join, extname } from 'path';

const router = Router();

/** 获取专辑封面 */
router.get('/:albumId', async (req, res) => {
  try {
    const albumId = parseInt(req.params.albumId as string);
    const album = await db.select().from(schema.album).where(eq(schema.album.id, albumId)).get();

    const library = await db.select().from(schema.mediaLibrary).limit(1).get();
    if (!library) { res.status(404).end(); return; }

    // 1. 尝试从数据库记录的 coverPath 读取
    if (album?.coverPath) {
      const fullPath = join(library.storagePath, album.coverPath);
      if (existsSync(fullPath)) {
        const stat = statSync(fullPath);
        res.setHeader('Content-Type', 'image/jpeg');
        res.setHeader('Content-Length', stat.size);
        res.setHeader('Cache-Control', 'public, max-age=86400');
        res.send(readFileSync(fullPath));
        return;
      }
    }

    // 2. 尝试在专辑目录下查找 cover.jpg / folder.jpg / front.jpg
    if (album?.artistId) {
      // 查找属于该专辑的任意一首歌，获取其目录路径
      const track = await db.select({ storagePath: schema.track.storagePath })
        .from(schema.track).where(eq(schema.track.albumId, albumId)).limit(1).get();
      if (track) {
        const trackDir = join(library.storagePath, track.storagePath.replace(/\\/[^\\/]*$/, ''));
        const coverNames = ['cover.jpg', 'cover.jpeg', 'cover.png', 'folder.jpg', 'folder.jpeg', 'front.jpg', 'front.jpeg'];
        for (const name of coverNames) {
          const coverFile = join(trackDir, name);
          if (existsSync(coverFile)) {
            const stat = statSync(coverFile);
            res.setHeader('Content-Type', 'image/jpeg');
            res.setHeader('Content-Length', stat.size);
            res.setHeader('Cache-Control', 'public, max-age=86400');
            res.send(readFileSync(coverFile));
            return;
          }
        }
      }
    }

    // 3. 返回默认封面
    res.setHeader('Content-Type', 'image/svg+xml');
    res.send('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">🎵</text></svg>');
  } catch {
    res.status(500).end();
  }
});

export default router;
