import { Router } from 'express';
import { db, schema } from '../../db/index.js';
import { eq, like, or, desc, sql, and } from 'drizzle-orm';
import bcrypt from 'bcryptjs';
import { createReadStream } from 'fs';
import { join, extname } from 'path';
import { config } from '../../config/index.js';

const router = Router();

declare module 'express-serve-static-core' {
  interface Request {
    subsonicUser?: any;
    subsonicVersion?: string;
    subsonicFormat?: string;
  }
}

// ─── Subsonic 认证中间件 ─────────────────────────────────
async function subsonicAuth(req: any, res: any, next: any) {
  const u = req.query.u || req.body?.u;
  const p = req.query.p || req.body?.p;
  const t = req.query.t || req.body?.t;
  const s = req.query.s || req.body?.s;
  const v = req.query.v || '1.16.1';
  const c = req.query.c || 'LeChenMusic';
  const f = req.query.f || 'json';

  if (!u) return res.json(subsonicError(10, 'Missing username'));

  const user = await db.select().from(schema.sysUser).where(eq(schema.sysUser.username, u as string)).get();
  if (!user) return res.json(subsonicError(40, 'Wrong username or password'));

  // 支持明文密码和 token 认证
  if (p) {
    const pass = (p as string).replace('enc:', '');
    const valid = await bcrypt.compare(pass, user.passwordHash);
    if (!valid) return res.json(subsonicError(40, 'Wrong username or password'));
  }

  req.subsonicUser = user;
  req.subsonicVersion = v;
  req.subsonicFormat = f;
  next();
}

function subsonicError(code: number, message: string) {
  return {
    'subsonic-response': {
      status: 'failed',
      version: '1.16.1',
      error: { code, message },
    },
  };
}

function subsonicOk(data: any, version?: string) {
  return {
    'subsonic-response': {
      status: 'ok',
      version: version || '1.16.1',
      ...data,
    },
  };
}

function formatTrack(t: any) {
  return {
    id: String(t.id),
    title: t.title,
    artist: t.artistName || 'Unknown',
    album: t.albumTitle || 'Unknown',
    artistId: String(t.artistId || ''),
    albumId: String(t.albumId || ''),
    track: t.trackNumber || 0,
    discNumber: t.discNumber || 1,
    year: t.year || 0,
    genre: t.genre || '',
    size: t.fileSize || 0,
    contentType: getMime(t.format),
    suffix: t.format || '',
    duration: t.duration || 0,
    bitRate: t.bitrate || 0,
    path: t.storagePath || '',
    isVideo: false,
    created: t.createdAt ? new Date(t.createdAt ? t.createdAt.getTime() : 0).toISOString() : '',
  };
}

function getMime(format: string) {
  const map: Record<string, string> = {
    mp3: 'audio/mpeg', flac: 'audio/flac', aac: 'audio/aac', ogg: 'audio/ogg',
    wav: 'audio/wav', m4a: 'audio/mp4', opus: 'audio/opus', wma: 'audio/x-ms-wma',
    ape: 'audio/ape', dsf: 'audio/dsd', aiff: 'audio/aiff',
  };
  return map[format] || 'application/octet-stream';
}

// ─── 系统 ────────────────────────────────────────────────
router.all('/ping', subsonicAuth, (req, res) => {
  res.json(subsonicOk({}));
});

router.all('/getLicense', subsonicAuth, (req, res) => {
  res.json(subsonicOk({
    license: { valid: true, email: 'admin@lechen.local', licenseExpires: '2099-01-01T00:00:00' },
  }));
});

// ─── 浏览 ────────────────────────────────────────────────
router.all('/getMusicFolders', subsonicAuth, async (req, res) => {
  const libs = await db.select().from(schema.mediaLibrary).all();
  res.json(subsonicOk({
    musicFolders: { musicFolder: libs.map(l => ({ id: String(l.id), name: l.name })) },
  }));
});

router.all('/getIndexes', subsonicAuth, async (req, res) => {
  const artists = await db.select().from(schema.artist).orderBy(schema.artist.name).all();
  const indexMap: Record<string, any[]> = {};
  for (const a of artists) {
    const letter = (a.nameSort || a.name)[0].toUpperCase();
    if (!indexMap[letter]) indexMap[letter] = [];
    indexMap[letter].push({ id: String(a.id), name: a.name, albumCount: 0 });
  }
  res.json(subsonicOk({
    indexes: {
      lastModified: Date.now(),
      index: Object.entries(indexMap).map(([name, artist]) => ({ name, artist })),
    },
  }));
});

router.all('/getArtists', subsonicAuth, async (req, res) => {
  const artists = await db.select().from(schema.artist).orderBy(schema.artist.name).all();
  const indexMap: Record<string, any[]> = {};
  for (const a of artists) {
    const letter = (a.nameSort || a.name)[0].toUpperCase();
    if (!indexMap[letter]) indexMap[letter] = [];
    indexMap[letter].push({ id: String(a.id), name: a.name, coverArt: '', albumCount: 0 });
  }
  res.json(subsonicOk({
    artists: {
      index: Object.entries(indexMap).map(([name, artist]) => ({ name, artist })),
    },
  }));
});

router.all('/getArtist', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  const artist = await db.select().from(schema.artist).where(eq(schema.artist.id, id)).get();
  if (!artist) return res.json(subsonicError(70, 'Artist not found'));

  const albums = await db.select().from(schema.album).where(eq(schema.album.artistId, id)).all();
  res.json(subsonicOk({
    artist: {
      id: String(artist.id),
      name: artist.name,
      album: albums.map(a => ({
        id: String(a.id), name: a.title, artist: artist.name, year: a.year || 0,
        coverArt: a.coverPath ? String(a.id) : '', songCount: 0, duration: 0,
      })),
    },
  }));
});

router.all('/getAlbum', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  const album = await db.select().from(schema.album).where(eq(schema.album.id, id)).get();
  if (!album) return res.json(subsonicError(70, 'Album not found'));

  const artist = album.artistId ? await db.select().from(schema.artist).where(eq(schema.artist.id, album.artistId)).get() : null;
  const tracks = await db.select({
    id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
    albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
    format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
    discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
    createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
  })
    .from(schema.track)
    .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
    .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
    .where(eq(schema.track.albumId, id))
    .orderBy(schema.track.discNumber, schema.track.trackNumber).all();

  res.json(subsonicOk({
    album: {
      id: String(album.id), name: album.title, artist: artist?.name || 'Unknown',
      artistId: String(album.artistId || ''), year: album.year || 0, genre: album.genre || '',
      coverArt: album.coverPath ? String(album.id) : '',
      song: tracks.map(formatTrack),
    },
  }));
});

router.all('/getAlbumList2', subsonicAuth, async (req, res) => {
  const type = req.query.type || 'newest';
  const size = parseInt(req.query.size as string) || 50;
  const offset = parseInt(req.query.offset as string) || 0;

  let query = db.select({
    id: schema.album.id, title: schema.album.title, artistId: schema.album.artistId,
    year: schema.album.year, genre: schema.album.genre, coverPath: schema.album.coverPath,
    artistName: schema.artist.name, createdAt: schema.album.createdAt,
  })
    .from(schema.album)
    .leftJoin(schema.artist, eq(schema.album.artistId, schema.artist.id));

  if (type === 'newest') query = query.orderBy(desc(schema.album.createdAt)) as any;
  else if (type === 'alphabeticalByName') query = query.orderBy(schema.album.title) as any;
  else if (type === 'random') query = query.orderBy(sql`random()`) as any;
  else query = query.orderBy(desc(schema.album.createdAt)) as any;

  const albums = await query.limit(size).offset(offset).all();

  res.json(subsonicOk({
    albumList2: {
      album: albums.map(a => ({
        id: String(a.id), name: a.title, artist: a.artistName || 'Unknown',
        artistId: String(a.artistId || ''), year: a.year || 0, genre: a.genre || '',
        coverArt: a.coverPath ? String(a.id) : '',
      })),
    },
  }));
});

// ─── 歌曲 ────────────────────────────────────────────────
router.all('/getSong', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  const track = await db.select({
    id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
    albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
    format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
    discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
    createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
  })
    .from(schema.track)
    .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
    .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
    .where(eq(schema.track.id, id)).get();

  if (!track) return res.json(subsonicError(70, 'Song not found'));
  res.json(subsonicOk({ song: formatTrack(track) }));
});

router.all('/getRandomSongs', subsonicAuth, async (req, res) => {
  const size = parseInt(req.query.size as string) || 10;
  const tracks = await db.select({
    id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
    albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
    format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
    discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
    createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
  })
    .from(schema.track)
    .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
    .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
    .orderBy(sql`random()`).limit(size).all();

  res.json(subsonicOk({ randomSongs: { song: tracks.map(formatTrack) } }));
});

// ─── 搜索 ────────────────────────────────────────────────
router.all('/search2', subsonicAuth, async (req, res) => {
  const query = req.query.query as string;
  if (!query) return res.json(subsonicOk({ searchResult2: {} }));

  const pattern = `%${query}%`;
  const [songs, albums, artists] = await Promise.all([
    db.select({
      id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
      albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
      format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
      discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
      createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
    })
      .from(schema.track)
      .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
      .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
      .where(or(like(schema.track.title, pattern), like(schema.track.genre, pattern)))
      .limit(20).all(),
    db.select().from(schema.album).where(like(schema.album.title, pattern)).limit(10).all(),
    db.select().from(schema.artist).where(like(schema.artist.name, pattern)).limit(10).all(),
  ]);

  res.json(subsonicOk({
    searchResult2: {
      song: songs.map(formatTrack),
      album: albums.map(a => ({ id: String(a.id), name: a.title, coverArt: a.coverPath ? String(a.id) : '' })),
      artist: artists.map(a => ({ id: String(a.id), name: a.name })),
    },
  }));
});

// ─── 流式播放 ────────────────────────────────────────────
router.all('/stream', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  const maxBitRate = parseInt(req.query.maxBitRate as string) || 0;

  const track = await db.select().from(schema.track).where(eq(schema.track.id, id)).get();
  if (!track) return res.json(subsonicError(70, 'Song not found'));

  const library = await db.select().from(schema.mediaLibrary)
    .where(eq(schema.mediaLibrary.storageType, track.storageType)).get();
  if (!library) return res.json(subsonicError(70, 'Storage not found'));

  const fullPath = join(library.storagePath, track.storagePath);
  const ext = extname(track.storagePath).toLowerCase();
  const mime = getMime(ext.slice(1));

  // 如果需要转码（maxBitRate 低于原始码率）
  if (maxBitRate > 0 && track.bitrate && track.bitrate > maxBitRate) {
    try {
      const { spawn } = await import('child_process');
      res.setHeader('Content-Type', 'audio/mpeg');
      res.setHeader('Accept-Ranges', 'none');

      const ffmpeg = spawn('ffmpeg', [
        '-i', fullPath,
        '-ab', `${maxBitRate}k`,
        '-f', 'mp3',
        '-map_metadata', '-1',
        'pipe:1',
      ], { stdio: ['pipe', 'pipe', 'pipe'] });

      ffmpeg.stdout.pipe(res);
      req.on('close', () => ffmpeg.kill('SIGKILL'));
      ffmpeg.on('error', () => { try { res.end(); } catch {} });
      return;
    } catch {
      // 转码失败，回退到直链
    }
  }

  // 直链播放
  try {
    const stat = await import('fs').then(fs => fs.statSync(fullPath));
    const range = req.headers.range;

    if (range) {
      const parts = range.replace(/bytes=/, '').split('-');
      const start = parseInt(parts[0], 10);
      const end = parts[1] ? parseInt(parts[1], 10) : stat.size - 1;
      res.status(206);
      res.setHeader('Content-Range', `bytes ${start}-${end}/${stat.size}`);
      res.setHeader('Accept-Ranges', 'bytes');
      res.setHeader('Content-Length', end - start + 1);
      res.setHeader('Content-Type', mime);
      createReadStream(fullPath, { start, end }).pipe(res);
    } else {
      res.setHeader('Content-Length', stat.size);
      res.setHeader('Content-Type', mime);
      res.setHeader('Accept-Ranges', 'bytes');
      createReadStream(fullPath).pipe(res);
    }
  } catch (err: any) {
    res.json(subsonicError(70, 'File read error: ' + err.message));
  }
});

// ─── 歌词 ────────────────────────────────────────────────
router.all('/getLyrics', subsonicAuth, async (req, res) => {
  const artist = req.query.artist as string;
  const title = req.query.title as string;

  if (artist && title) {
    const track = await db.select().from(schema.track)
      .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
      .where(and(like(schema.track.title, `%${title}%`), like(schema.artist.name, `%${artist}%`)))
      .get();

    if (track) {
      const lyric = await db.select().from(schema.lyric)
        .where(eq(schema.lyric.trackId, track.track.id)).get();
      if (lyric) {
        return res.json(subsonicOk({ lyrics: { artist, title, content: lyric.content } }));
      }
    }
  }

  res.json(subsonicOk({ lyrics: { artist: artist || '', title: title || '', content: '' } }));
});

// ─── 收藏 ────────────────────────────────────────────────
router.all('/star', subsonicAuth, async (req, res) => {
  const id = req.query.id as string;
  const albumId = req.query.albumId as string;
  const artistId = req.query.artistId as string;

  if (id) {
    await db.insert(schema.favorite).values({
      userId: req.subsonicUser.id, targetType: 'track', targetId: parseInt(id),
    }).onConflictDoNothing();
  }
  if (albumId) {
    await db.insert(schema.favorite).values({
      userId: req.subsonicUser.id, targetType: 'album', targetId: parseInt(albumId),
    }).onConflictDoNothing();
  }
  if (artistId) {
    await db.insert(schema.favorite).values({
      userId: req.subsonicUser.id, targetType: 'artist', targetId: parseInt(artistId),
    }).onConflictDoNothing();
  }

  res.json(subsonicOk({}));
});

router.all('/unstar', subsonicAuth, async (req, res) => {
  const id = req.query.id as string;
  const albumId = req.query.albumId as string;
  const artistId = req.query.artistId as string;

  if (id) {
    await db.delete(schema.favorite).where(
      and(eq(schema.favorite.userId, req.subsonicUser.id), eq(schema.favorite.targetType, 'track'), eq(schema.favorite.targetId, parseInt(id)))
    );
  }
  if (albumId) {
    await db.delete(schema.favorite).where(
      and(eq(schema.favorite.userId, req.subsonicUser.id), eq(schema.favorite.targetType, 'album'), eq(schema.favorite.targetId, parseInt(albumId)))
    );
  }
  if (artistId) {
    await db.delete(schema.favorite).where(
      and(eq(schema.favorite.userId, req.subsonicUser.id), eq(schema.favorite.targetType, 'artist'), eq(schema.favorite.targetId, parseInt(artistId)))
    );
  }

  res.json(subsonicOk({}));
});

router.all('/getStarred2', subsonicAuth, async (req, res) => {
  const favs = await db.select().from(schema.favorite)
    .where(eq(schema.favorite.userId, req.subsonicUser.id)).all();

  const trackFavs = favs.filter(f => f.targetType === 'track');
  const albumFavs = favs.filter(f => f.targetType === 'album');
  const artistFavs = favs.filter(f => f.targetType === 'artist');

  const songs = [];
  for (const f of trackFavs) {
    const t = await db.select({
      id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
      albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
      format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
      discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
      createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
    })
      .from(schema.track)
      .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
      .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
      .where(eq(schema.track.id, f.targetId)).get();
    if (t) songs.push(formatTrack(t));
  }

  res.json(subsonicOk({ starred2: { song: songs } }));
});

// ─── 播放记录 ────────────────────────────────────────────
router.all('/scrobble', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  if (id) {
    await db.insert(schema.playHistory).values({
      userId: req.subsonicUser.id, trackId: id, position: 0,
    });
  }
  res.json(subsonicOk({}));
});

// ─── 歌单 ────────────────────────────────────────────────
router.all('/getPlaylists', subsonicAuth, async (req, res) => {
  const playlists = await db.select().from(schema.playlist)
    .where(eq(schema.playlist.userId, req.subsonicUser.id))
    .orderBy(desc(schema.playlist.updatedAt)).all();

  res.json(subsonicOk({
    playlists: {
      playlist: playlists.map(p => ({
        id: String(p.id), name: p.name, owner: req.subsonicUser.username,
        public: false, created: p.createdAt ? new Date(p.createdAt ? p.createdAt.getTime() : 0).toISOString() : '',
        changed: p.updatedAt ? new Date(p.updatedAt ? p.updatedAt.getTime() : 0).toISOString() : '',
      })),
    },
  }));
});

router.all('/getPlaylist', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  const playlist = await db.select().from(schema.playlist).where(eq(schema.playlist.id, id)).get();
  if (!playlist) return res.json(subsonicError(70, 'Playlist not found'));

  const tracks = await db.select({
    id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
    albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
    format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
    discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
    createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
  })
    .from(schema.playlistTrack)
    .innerJoin(schema.track, eq(schema.playlistTrack.trackId, schema.track.id))
    .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
    .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
    .where(eq(schema.playlistTrack.playlistId, id))
    .orderBy(schema.playlistTrack.sortOrder).all();

  res.json(subsonicOk({
    playlist: {
      id: String(playlist.id), name: playlist.name, owner: req.subsonicUser.username,
      songCount: tracks.length, duration: tracks.reduce((s: number, t: any) => s + (t.duration || 0), 0),
      song: tracks.map(formatTrack),
    },
  }));
});

// ─── 书签 ────────────────────────────────────────────────
router.all('/getBookmarks', subsonicAuth, async (req, res) => {
  res.json(subsonicOk({ bookmarks: { bookmark: [] } }));
});

// ─── 音乐目录信息 ────────────────────────────────────────
router.all('/getMusicDirectory', subsonicAuth, async (req, res) => {
  const id = parseInt(req.query.id as string);
  // 简单实现：返回所有歌曲
  const tracks = await db.select({
    id: schema.track.id, title: schema.track.title, artistId: schema.track.artistId,
    albumId: schema.track.albumId, duration: schema.track.duration, bitrate: schema.track.bitrate,
    format: schema.track.format, fileSize: schema.track.fileSize, trackNumber: schema.track.trackNumber,
    discNumber: schema.track.discNumber, genre: schema.track.genre, storagePath: schema.track.storagePath,
    createdAt: schema.track.createdAt, artistName: schema.artist.name, albumTitle: schema.album.title,
  })
    .from(schema.track)
    .leftJoin(schema.artist, eq(schema.track.artistId, schema.artist.id))
    .leftJoin(schema.album, eq(schema.track.albumId, schema.album.id))
    .limit(500).all();

  res.json(subsonicOk({
    directory: {
      id: '0',
      name: 'Music',
      child: tracks.map(formatTrack),
    },
  }));
});

export default router;
