import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { existsSync, readFileSync, statSync, mkdirSync, writeFileSync } from 'fs';
import { join, dirname, basename, extname, relative } from 'path';
// @ts-expect-error
import { parseFile } from 'music-metadata';

const router = Router();

/** 从音频文件内嵌封面中提取并返回图片 */
async function extractCoverFromAudio(audioFullPath: string): Promise<Buffer | null> {
  try {
    if (!existsSync(audioFullPath)) return null;
    const metadata = await parseFile(audioFullPath, { skipCovers: false, duration: false });
    if (metadata.common.picture && metadata.common.picture.length > 0) {
      const pic = metadata.common.picture[0];
      if (pic.data && pic.data.length > 0) {
        return Buffer.from(pic.data);
      }
    }
  } catch (err: any) {
    // Log error for debugging (only in dev)
    if (process.env.NODE_ENV !== 'production') {
      console.warn(`Failed to extract cover from ${audioFullPath}:`, err.message);
    }
  }
  return null;
}

/** 尝试多种方式获取专辑封面，返回 Buffer 或 null */
async function getAlbumCover(albumId: number): Promise<{ buf: Buffer; mime: string } | null> {
  const album = await db.select().from(schema.album).where(eq(schema.album.id, albumId)).get();

  // 获取媒体库路径
  const library = await db.select().from(schema.mediaLibrary).limit(1).get();
  if (!library) return null;

  // 1. 从数据库 coverPath 读取
  if (album?.coverPath) {
    const fullPath = join(library.storagePath, album.coverPath);
    if (existsSync(fullPath)) {
      const ext = extname(fullPath).toLowerCase();
      const mime = ext === '.png' ? 'image/png' : 'image/jpeg';
      return { buf: readFileSync(fullPath), mime };
    }
  }

  // 2. 查找专辑下任意一首歌的目录，尝试目录封面文件
  const track = await db.select({ storagePath: schema.track.storagePath })
    .from(schema.track).where(eq(schema.track.albumId, albumId)).limit(1).get();
  if (track) {
    const trackDir = join(library.storagePath, dirname(track.storagePath));
    const coverNames = ['cover.jpg', 'cover.jpeg', 'cover.png', 'folder.jpg', 'folder.jpeg', 'front.jpg', 'front.jpeg'];
    for (const name of coverNames) {
      const coverFile = join(trackDir, name);
      if (existsSync(coverFile)) {
        const ext = extname(coverFile).toLowerCase();
        const mime = ext === '.png' ? 'image/png' : 'image/jpeg';
        return { buf: readFileSync(coverFile), mime };
      }
    }

    // 3. 从音频文件内嵌封面提取
    // 尝试多首歌，提高找到封面的概率
    const tracks = await db.select({ storagePath: schema.track.storagePath })
      .from(schema.track).where(eq(schema.track.albumId, albumId)).limit(5).all();
    for (const t of tracks) {
      const audioFullPath = join(library.storagePath, t.storagePath);
      if (existsSync(audioFullPath)) {
        const coverBuf = await extractCoverFromAudio(audioFullPath);
        if (coverBuf) {
          // 同时保存到 .covers 目供后续快速读取
          try {
            const trackDir = join(library.storagePath, dirname(t.storagePath));
            const coverDir = join(trackDir, '.covers');
            const coverName = `${basename(t.storagePath, extname(t.storagePath))}.jpg`;
            const coverPath = join(coverDir, coverName);
            if (!existsSync(coverPath)) {
              mkdirSync(coverDir, { recursive: true });
              writeFileSync(coverPath, coverBuf);
            }
            // 更新数据库
            const relCover = relative(library.storagePath, coverPath);
            await db.update(schema.album).set({ coverPath: relCover }).where(eq(schema.album.id, albumId));
          } catch {
            // 即使保存失败也返回封面
          }
          return { buf: coverBuf, mime: 'image/jpeg' };
        }
      }
    }
  }

  return null;
}

/** 获取专辑封面 */
router.get('/:albumId', async (req, res) => {
  try {
    const albumId = parseInt(req.params.albumId as string);
    const result = await getAlbumCover(albumId);
    if (result) {
      res.setHeader('Content-Type', result.mime);
      res.setHeader('Content-Length', result.buf.length);
      res.setHeader('Cache-Control', 'public, max-age=86400');
      res.send(result.buf);
      return;
    }
    // 默认封面
    res.setHeader('Content-Type', 'image/svg+xml');
    res.send('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">🎵</text></svg>');
  } catch {
    res.status(500).end();
  }
});

/** 获取歌曲封面（直接从音频文件提取） */
router.get('/track/:trackId', async (req, res) => {
  try {
    const trackId = parseInt(req.params.trackId as string);
    const track = await db.select({
      storagePath: schema.track.storagePath,
      albumId: schema.track.albumId,
    }).from(schema.track).where(eq(schema.track.id, trackId)).get();

    if (!track) { res.status(404).end(); return; }

    // 优先用专辑封面
    if (track.albumId) {
      const result = await getAlbumCover(track.albumId);
      if (result) {
        res.setHeader('Content-Type', result.mime);
        res.setHeader('Content-Length', result.buf.length);
        res.setHeader('Cache-Control', 'public, max-age=86400');
        res.send(result.buf);
        return;
      }
    }

    // 直接从该音频文件提取
    const library = await db.select().from(schema.mediaLibrary).limit(1).get();
    if (library) {
      const audioFullPath = join(library.storagePath, track.storagePath);
      if (existsSync(audioFullPath)) {
        const coverBuf = await extractCoverFromAudio(audioFullPath);
        if (coverBuf) {
          res.setHeader('Content-Type', 'image/jpeg');
          res.setHeader('Content-Length', coverBuf.length);
          res.setHeader('Cache-Control', 'public, max-age=86400');
          res.send(coverBuf);
          return;
        }
      }
    }

    // 默认
    res.setHeader('Content-Type', 'image/svg+xml');
    res.send('<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">🎵</text></svg>');
  } catch {
    res.status(500).end();
  }
});

export default router;
