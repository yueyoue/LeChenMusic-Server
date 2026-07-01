import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
import { existsSync, readFileSync, statSync, mkdirSync, writeFileSync } from 'fs';
import { join, dirname, basename, extname, relative } from 'path';
// @ts-expect-error
import { parseFile } from 'music-metadata';

const router = Router();

/** 从音频文件内嵌封面提取 */
async function extractEmbeddedCover(audioPath: string): Promise<Buffer | null> {
  try {
    if (!existsSync(audioPath)) return null;
    const meta = await parseFile(audioPath, { skipCovers: false, duration: false });
    if (meta.common.picture?.length > 0 && meta.common.picture[0].data?.length > 0) {
      return Buffer.from(meta.common.picture[0].data);
    }
  } catch { /* ignore */ }
  return null;
}

/** 查找目录下的封面文件 */
function findCoverFile(dir: string): string | null {
  const names = ['cover.jpg', 'cover.jpeg', 'cover.png', 'folder.jpg', 'folder.jpeg', 'front.jpg', 'front.jpeg', 'album.jpg', 'album.jpeg'];
  for (const name of names) {
    const p = join(dir, name);
    if (existsSync(p)) return p;
  }
  return null;
}

/** 获取媒体库路径 */
async function getLibPath(): Promise<string | null> {
  const lib = await db.select().from(schema.mediaLibrary).limit(1).get();
  return lib?.storagePath ?? null;
}

/** 歌曲封面：优先从该音频文件自身提取 */
router.get('/track/:trackId', async (req, res) => {
  try {
    const trackId = parseInt(req.params.trackId as string);
    const track = await db.select({
      storagePath: schema.track.storagePath,
      albumId: schema.track.albumId,
    }).from(schema.track).where(eq(schema.track.id, trackId)).get();

    if (!track) { res.status(404).end(); return; }

    const libPath = await getLibPath();
    if (!libPath) { res.status(404).end(); return; }

    const audioFullPath = join(libPath, track.storagePath);
    const trackDir = dirname(audioFullPath);

    // 1. 优先：从该音频文件自身的内嵌封面提取
    const embedded = await extractEmbeddedCover(audioFullPath);
    if (embedded) {
      res.setHeader('Content-Type', 'image/jpeg');
      res.setHeader('Content-Length', embedded.length);
      res.setHeader('Cache-Control', 'public, max-age=86400');
      res.send(embedded);
      return;
    }

    // 2. 该歌曲目录下的封面文件
    const coverFile = findCoverFile(trackDir);
    if (coverFile) {
      const stat = statSync(coverFile);
      const mime = extname(coverFile).toLowerCase() === '.png' ? 'image/png' : 'image/jpeg';
      res.setHeader('Content-Type', mime);
      res.setHeader('Content-Length', stat.size);
      res.setHeader('Cache-Control', 'public, max-age=86400');
      res.send(readFileSync(coverFile));
      return;
    }

    // 3. 最后才用专辑封面
    if (track.albumId) {
      const albumCover = await getAlbumCoverBuf(track.albumId, libPath);
      if (albumCover) {
        res.setHeader('Content-Type', albumCover.mime);
        res.setHeader('Content-Length', albumCover.buf.length);
        res.setHeader('Cache-Control', 'public, max-age=86400');
        res.send(albumCover.buf);
        return;
      }
    }

    // 4. 默认
    sendDefaultCover(res, '🎵');
  } catch (err) {
    console.error('Cover track error:', err);
    res.status(500).end();
  }
});

/** 专辑封面 */
async function getAlbumCoverBuf(albumId: number, libPath: string): Promise<{ buf: Buffer; mime: string } | null> {
  const album = await db.select().from(schema.album).where(eq(schema.album.id, albumId)).get();

  // 1. 数据库 coverPath
  if (album?.coverPath) {
    const fullPath = join(libPath, album.coverPath);
    if (existsSync(fullPath)) {
      const mime = extname(fullPath).toLowerCase() === '.png' ? 'image/png' : 'image/jpeg';
      return { buf: readFileSync(fullPath), mime };
    }
  }

  // 2. 专辑下任意歌曲的目录封面文件
  const tracks = await db.select({ storagePath: schema.track.storagePath })
    .from(schema.track).where(eq(schema.track.albumId, albumId)).limit(5).all();

  for (const t of tracks) {
    const trackDir = join(libPath, dirname(t.storagePath));
    const coverFile = findCoverFile(trackDir);
    if (coverFile) {
      const mime = extname(coverFile).toLowerCase() === '.png' ? 'image/png' : 'image/jpeg';
      return { buf: readFileSync(coverFile), mime };
    }
  }

  // 3. 从歌曲内嵌封面提取
  for (const t of tracks) {
    const audioPath = join(libPath, t.storagePath);
    const embedded = await extractEmbeddedCover(audioPath);
    if (embedded) {
      // 缓存到 .covers
      try {
        const trackDir = join(libPath, dirname(t.storagePath));
        const coverDir = join(trackDir, '.covers');
        const coverName = `${basename(t.storagePath, extname(t.storagePath))}.jpg`;
        const coverPath = join(coverDir, coverName);
        if (!existsSync(coverPath)) {
          mkdirSync(coverDir, { recursive: true });
          writeFileSync(coverPath, embedded);
        }
        const relCover = relative(libPath, coverPath);
        await db.update(schema.album).set({ coverPath: relCover }).where(eq(schema.album.id, albumId));
      } catch { /* ignore */ }
      return { buf: embedded, mime: 'image/jpeg' };
    }
  }

  return null;
}

router.get('/:albumId', async (req, res) => {
  try {
    const albumId = parseInt(req.params.albumId as string);
    const libPath = await getLibPath();
    if (!libPath) { sendDefaultCover(res, '💿'); return; }

    const result = await getAlbumCoverBuf(albumId, libPath);
    if (result) {
      res.setHeader('Content-Type', result.mime);
      res.setHeader('Content-Length', result.buf.length);
      res.setHeader('Cache-Control', 'public, max-age=86400');
      res.send(result.buf);
      return;
    }

    sendDefaultCover(res, '💿');
  } catch (err) {
    console.error('Cover album error:', err);
    res.status(500).end();
  }
});

function sendDefaultCover(res: any, emoji: string) {
  res.setHeader('Content-Type', 'image/svg+xml');
  res.send(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200"><rect fill="#1e293b" width="200" height="200"/><text fill="#64748b" font-size="48" x="50%" y="50%" text-anchor="middle" dy=".3em">${emoji}</text></svg>`);
}

export default router;
