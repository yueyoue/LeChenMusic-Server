import { db, schema } from '../../db/index.js';
import { eq, like, or, desc, asc, sql, and } from 'drizzle-orm';
import { storageManager } from '../storage/storage-manager.js';
import type { StorageType } from '../storage/types.js';
import { createReadStream } from 'fs';
import { join } from 'path';
import { extname } from 'path';
import { AppError } from '../../middleware/error-handler.js';

const MIME_MAP: Record<string, string> = {
  '.mp3': 'audio/mpeg',
  '.flac': 'audio/flac',
  '.aac': 'audio/aac',
  '.ogg': 'audio/ogg',
  '.wav': 'audio/wav',
  '.m4a': 'audio/mp4',
  '.opus': 'audio/opus',
  '.wma': 'audio/x-ms-wma',
  '.ape': 'audio/ape',
  '.dsf': 'audio/dsd',
  '.aiff': 'audio/aiff',
};

export class TrackService {
  async list(options: {
    page?: number;
    pageSize?: number;
    search?: string;
    artistId?: number;
    albumId?: number;
    genre?: string;
    orderBy?: 'title' | 'createdAt' | 'trackNumber';
    order?: 'asc' | 'desc';
  }) {
    const { page = 1, pageSize = 50, search, artistId, albumId, genre, orderBy = 'title', order = 'asc' } = options;

    const conditions = [];
    if (search) {
      conditions.push(or(
        like(schema.track.title, `%${search}%`),
        like(schema.track.genre, `%${search}%`),
      ));
    }
    if (artistId) conditions.push(eq(schema.track.artistId, artistId));
    if (albumId) conditions.push(eq(schema.track.albumId, albumId));
    if (genre) conditions.push(eq(schema.track.genre, genre));

    const where = conditions.length > 0 ? and(...conditions) : undefined;

    const orderFn = order === 'asc' ? asc : desc;
    const orderCol = orderBy === 'createdAt' ? schema.track.createdAt
      : orderBy === 'trackNumber' ? schema.track.trackNumber
      : schema.track.title;

    const [items, total] = await Promise.all([
      db.select({
        id: schema.track.id,
        title: schema.track.title,
        artistId: schema.track.artistId,
        albumId: schema.track.albumId,
        duration: schema.track.duration,
        bitrate: schema.track.bitrate,
        bitDepth: schema.track.bitDepth,
        sampleRate: schema.track.sampleRate,
        format: schema.track.format,
        fileSize: schema.track.fileSize,
        trackNumber: schema.track.trackNumber,
        discNumber: schema.track.discNumber,
        genre: schema.track.genre,
      })
        .from(schema.track)
        .where(where)
        .orderBy(orderFn(orderCol))
        .limit(pageSize)
        .offset((page - 1) * pageSize)
        .all(),
      db.select({ count: sql<number>`count(*)` })
        .from(schema.track)
        .where(where)
        .get()
        .then(r => r?.count ?? 0),
    ]);

    return { items, total, page, pageSize };
  }

  async getById(id: number) {
    const track = await db.select().from(schema.track).where(eq(schema.track.id, id)).get();
    if (!track) throw new AppError(3001, 404, 'Track not found');
    return track;
  }

  async getStreamInfo(id: number) {
    const track = await this.getById(id);

    const library = await db.select().from(schema.mediaLibrary)
      .where(eq(schema.mediaLibrary.storageType, track.storageType)).get();

    if (!library) throw new AppError(3002, 404, 'Storage library not found');

    const driver = storageManager.getDriver(
      track.storageType as StorageType,
      library.storagePath,
    );

    const fullPath = join(library.storagePath, track.storagePath);
    const stat = await driver.stat(track.storagePath);
    const ext = extname(track.storagePath).toLowerCase();
    const contentType = MIME_MAP[ext] || 'application/octet-stream';

    return {
      track,
      driver,
      stat,
      contentType,
      fullPath,
    };
  }

  async stream(id: number, rangeHeader: string | undefined) {
    const { track, stat, contentType, fullPath } = await this.getStreamInfo(id);

    if (rangeHeader) {
      const parts = rangeHeader.replace(/bytes=/, '').split('-');
      const start = parseInt(parts[0], 10);
      const end = parts[1] ? parseInt(parts[1], 10) : stat.size - 1;
      const chunkSize = end - start + 1;

      const stream = createReadStream(fullPath, { start, end });
      return {
        stream,
        headers: {
          'Content-Range': `bytes ${start}-${end}/${stat.size}`,
          'Accept-Ranges': 'bytes',
          'Content-Length': chunkSize,
          'Content-Type': contentType,
        },
        status: 206,
      };
    }

    const stream = createReadStream(fullPath);
    return {
      stream,
      headers: {
        'Accept-Ranges': 'bytes',
        'Content-Length': stat.size,
        'Content-Type': contentType,
      },
      status: 200,
    };
  }
}

export const trackService = new TrackService();
