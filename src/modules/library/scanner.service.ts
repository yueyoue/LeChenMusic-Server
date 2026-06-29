import { db, schema } from '../../db/index.js';
import { eq, and } from 'drizzle-orm';
// @ts-ignore - music-metadata ESM export map resolution workaround
import { parseFile } from 'music-metadata/lib/index.js';
import { createHash } from 'crypto';
import { readFileSync, existsSync } from 'fs';
import { join, extname, basename, dirname, relative } from 'path';
import { storageManager } from '../storage/storage-manager.js';
import type { StorageType } from '../storage/types.js';
import { logger } from '../../utils/logger.js';

const AUDIO_EXTENSIONS = new Set([
  '.mp3', '.flac', '.aac', '.ogg', '.wav', '.wma', '.ape', '.dsf', '.dff',
  '.m4a', '.opus', '.alac', '.aiff',
]);

export class ScannerService {
  private scanning = new Set<number>();

  async scanLibrary(libraryId: number) {
    if (this.scanning.has(libraryId)) {
      throw new Error('Scan already in progress');
    }

    this.scanning.add(libraryId);

    try {
      // 更新扫描状态
      await db.update(schema.mediaLibrary)
        .set({ scanStatus: 'scanning' })
        .where(eq(schema.mediaLibrary.id, libraryId));

      const library = await db.select().from(schema.mediaLibrary)
        .where(eq(schema.mediaLibrary.id, libraryId)).get();

      if (!library) throw new Error('Library not found');

      const driver = storageManager.getDriver(
        library.storageType as StorageType,
        library.storagePath,
      );

      let scannedCount = 0;
      const errors: string[] = [];

      // 递归扫描目录
      await this.scanDir(driver, library, '', libraryId, (count, errs) => {
        scannedCount = count;
        errors.push(...errs);
      });

      // 更新扫描结果
      await db.update(schema.mediaLibrary)
        .set({
          scanStatus: errors.length > 0 ? 'error' : 'idle',
          lastScanAt: new Date(),
          fileCount: scannedCount,
        })
        .where(eq(schema.mediaLibrary.id, libraryId));

      logger.info({ libraryId, scannedCount, errors: errors.length }, 'Library scan completed');

      return { scannedCount, errors };
    } finally {
      this.scanning.delete(libraryId);
    }
  }

  private async scanDir(
    driver: Awaited<ReturnType<typeof storageManager.getDriver>>,
    library: typeof schema.mediaLibrary.$inferSelect,
    relPath: string,
    libraryId: number,
    onProgress: (count: number, errors: string[]) => void,
  ) {
    const items = await driver.listDir(relPath);
    let count = 0;
    const errors: string[] = [];

    for (const item of items) {
      if (item.isDirectory) {
        // 递归扫描子目录
        const sub = await this.scanDir(driver, library, item.path, libraryId, onProgress);
        count += sub.count;
        errors.push(...sub.errors);
      } else if (item.isFile && AUDIO_EXTENSIONS.has(extname(item.name).toLowerCase())) {
        try {
          await this.processAudioFile(driver, library, item.path, libraryId);
          count++;
          if (count % 100 === 0) {
            onProgress(count, errors);
          }
        } catch (err: any) {
          const msg = `Failed to process ${item.path}: ${err.message}`;
          errors.push(msg);
          logger.warn({ path: item.path, err: err.message }, 'Failed to process audio file');
        }
      }
    }

    onProgress(count, errors);
    return { count, errors };
  }

  private async processAudioFile(
    driver: Awaited<ReturnType<typeof storageManager.getDriver>>,
    library: typeof schema.mediaLibrary.$inferSelect,
    relPath: string,
    libraryId: number,
  ) {
    const fileStat = await driver.stat(relPath);
    const stream = await driver.createReadStream(relPath);

    // 解析音频元数据
    const metadata = await parseFile(join(library.storagePath, relPath), {
      duration: true,
      skipCovers: false,
    });

    // 计算文件哈希（用文件大小 + 修改时间的简化哈希，避免全文件读取）
    const hashInput = `${relPath}:${fileStat.size}:${fileStat.mtime.getTime()}`;
    const fileHash = createHash('md5').update(hashInput).digest('hex');

    // 检查是否已存在（基于哈希去重）
    const existing = await db.select().from(schema.track)
      .where(eq(schema.track.fileHash, fileHash)).get();

    if (existing) return; // 已存在，跳过

    const common = metadata.common;
    const format = metadata.format;

    // 处理艺人
    let artistId: number | null = null;
    if (common.artist) {
      artistId = await this.findOrCreateArtist(common.artist, common.albumartist);
    }

    // 处理专辑
    let albumId: number | null = null;
    if (common.album) {
      albumId = await this.findOrCreateAlbum(common.album, artistId, common);
    }

    // 提取封面
    let coverPath: string | null = null;
    if (common.picture && common.picture.length > 0) {
      coverPath = await this.saveCover(common.picture[0], library.storagePath, relPath);
    }

    // 插入音轨记录
    await db.insert(schema.track).values({
      title: common.title || basename(relPath, extname(relPath)),
      artistId,
      albumId,
      duration: format.duration ? Math.round(format.duration) : null,
      bitrate: format.bitrate ? Math.round(format.bitrate / 1000) : null,
      bitDepth: format.bitsPerSample || null,
      sampleRate: format.sampleRate || null,
      format: extname(relPath).slice(1).toLowerCase(),
      fileSize: fileStat.size,
      trackNumber: common.track?.no || null,
      discNumber: common.disk?.no || 1,
      genre: common.genre?.[0] || null,
      storageType: library.storageType as StorageType,
      storagePath: relPath,
      fileHash,
    });

    // 如果有封面，更新专辑封面
    if (coverPath && albumId) {
      await db.update(schema.album)
        .set({ coverPath })
        .where(eq(schema.album.id, albumId));
    }
  }

  private async findOrCreateArtist(name: string, albumArtist?: string): Promise<number> {
    const artistName = albumArtist || name;
    const existing = await db.select().from(schema.artist)
      .where(eq(schema.artist.name, artistName)).get();

    if (existing) return existing.id;

    const result = await db.insert(schema.artist).values({
      name: artistName,
      nameSort: artistName.replace(/^(The|A|An)\s+/i, ''),
    }).returning().get();

    return result.id;
  }

  private async findOrCreateAlbum(
    title: string,
    artistId: number | null,
    common: any,
  ): Promise<number> {
    const existing = await db.select().from(schema.album)
      .where(
        artistId
          ? and(eq(schema.album.title, title), eq(schema.album.artistId, artistId))
          : eq(schema.album.title, title)
      ).get();

    if (existing) return existing.id;

    const result = await db.insert(schema.album).values({
      title,
      artistId,
      year: common.year || null,
      genre: common.genre?.[0] || null,
      totalTracks: common.track?.of || null,
      totalDiscs: common.disk?.of || 1,
    }).returning().get();

    return result.id;
  }

  private async saveCover(picture: any, basePath: string, filePath: string): Promise<string> {
    // 封面保存在音频文件同目录下的 .covers 文件夹
    const dir = dirname(filePath);
    const coverDir = join(basePath, dir, '.covers');
    const coverName = `${basename(filePath, extname(filePath))}.jpg`;
    const coverPath = join(coverDir, coverName);

    if (existsSync(coverPath)) return relative(basePath, coverPath);

    try {
      const { mkdirSync, writeFileSync } = await import('fs');
      mkdirSync(coverDir, { recursive: true });
      writeFileSync(coverPath, picture.data);
      return relative(basePath, coverPath);
    } catch {
      return '';
    }
  }
}

export const scannerService = new ScannerService();
