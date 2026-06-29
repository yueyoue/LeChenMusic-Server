import { db, schema } from '../../db/index.js';
import { eq } from 'drizzle-orm';
// @ts-expect-error - TypeScript resolves to 'core' entry but Node.js runtime uses 'node' entry which exports parseFile
import { parseFile } from 'music-metadata';
import { createHash } from 'crypto';
import { statSync, existsSync, readdirSync } from 'fs';
import { join, extname, basename } from 'path';
import { logger } from '../../utils/logger.js';

const AUDIO_EXTENSIONS = new Set([
  '.mp3', '.flac', '.aac', '.ogg', '.wav', '.m4a', '.opus', '.wma', '.ape',
]);

export class AudiobookScannerService {
  private scanning = new Set<number>();

  async scanLibrary(libraryId: number) {
    if (this.scanning.has(libraryId)) throw new Error('Scan already in progress');
    this.scanning.add(libraryId);

    try {
      await db.update(schema.mediaLibrary)
        .set({ scanStatus: 'scanning' })
        .where(eq(schema.mediaLibrary.id, libraryId));

      const library = await db.select().from(schema.mediaLibrary)
        .where(eq(schema.mediaLibrary.id, libraryId)).get();
      if (!library) throw new Error('Library not found');

      let scannedCount = 0;
      const errors: string[] = [];

      // 扫描有声书目录：每个子目录 = 一本有声书
      const bookDirs = readdirSync(library.storagePath, { withFileTypes: true })
        .filter(d => d.isDirectory());

      for (const bookDir of bookDirs) {
        try {
          await this.processAudiobook(library, bookDir.name);
          scannedCount++;
        } catch (err: any) {
          const msg = `Failed to process ${bookDir.name}: ${err.message}`;
          errors.push(msg);
          logger.warn({ path: bookDir.name, err: err.message }, 'Failed to process audiobook');
        }
      }

      await db.update(schema.mediaLibrary)
        .set({
          scanStatus: errors.length > 0 ? 'error' : 'idle',
          lastScanAt: new Date(),
          fileCount: scannedCount,
        })
        .where(eq(schema.mediaLibrary.id, libraryId));

      logger.info({ libraryId, scannedCount, errors: errors.length }, 'Audiobook scan completed');
      return { scannedCount, errors };
    } finally {
      this.scanning.delete(libraryId);
    }
  }

  private async processAudiobook(library: typeof schema.mediaLibrary.$inferSelect, bookDirName: string) {
    const bookPath = join(library.storagePath, bookDirName);
    const relPath = bookDirName;

    // 检查是否已存在
    const existing = await db.select().from(schema.audiobook)
      .where(eq(schema.audiobook.storagePath, relPath)).get();
    if (existing) return;

    // 扫描目录中的音频文件
    const files = readdirSync(bookPath, { withFileTypes: true })
      .filter(f => f.isFile() && AUDIO_EXTENSIONS.has(extname(f.name).toLowerCase()))
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));

    if (files.length === 0) return;

    // 从第一个文件提取元数据
    const firstFile = join(bookPath, files[0].name);
    let metadata: any = {};
    try {
      metadata = await parseFile(firstFile, { duration: true, skipCovers: false });
    } catch {}

    // 查找封面
    let coverPath: string | null = null;
    const coverFiles = ['cover.jpg', 'cover.jpeg', 'cover.png', 'folder.jpg', 'folder.jpeg'];
    for (const cf of coverFiles) {
      if (existsSync(join(bookPath, cf))) {
        coverPath = join(relPath, cf);
        break;
      }
    }
    // 如果没有单独封面文件，尝试从音频文件提取
    if (!coverPath && metadata.common?.picture?.[0]) {
      const pic = metadata.common.picture[0];
      const coverFile = join(bookPath, 'cover.jpg');
      try {
        const { writeFileSync } = await import('fs');
        writeFileSync(coverFile, pic.data);
        coverPath = join(relPath, 'cover.jpg');
      } catch {}
    }

    // 从目录名或元数据提取书名和作者
    const title = metadata.common?.album || bookDirName;
    const author = metadata.common?.albumartist || metadata.common?.artist || null;
    const genre = metadata.common?.genre?.[0] || null;
    const year = metadata.common?.year || null;

    // 插入有声书记录
    const book = await db.insert(schema.audiobook).values({
      title,
      author,
      narrator: null,
      coverPath,
      description: null,
      genre,
      year,
      chapterCount: files.length,
      storageType: library.storageType as any,
      storagePath: relPath,
    }).returning().get();

    // 插入章节
    let totalDuration = 0;
    for (let i = 0; i < files.length; i++) {
      const file = files[i];
      const filePath = join(bookPath, file.name);
      const relFilePath = join(relPath, file.name);

      let duration = 0;
      let fileSize = 0;
      let format = extname(file.name).slice(1).toLowerCase();
      let fileHash = '';

      try {
        const stat = statSync(filePath);
        fileSize = stat.size;
        fileHash = createHash('md5').update(`${relFilePath}:${stat.size}:${stat.mtimeMs}`).digest('hex');
      } catch {}

      try {
        const meta = await parseFile(filePath, { duration: true });
        duration = meta.format.duration ? Math.round(meta.format.duration) : 0;
      } catch {}

      totalDuration += duration;

      // 章节名：去掉扩展名
      const chapterTitle = basename(file.name, extname(file.name));

      await db.insert(schema.audiobookChapter).values({
        audiobookId: book.id,
        title: chapterTitle,
        chapterNumber: i + 1,
        duration,
        format,
        fileSize,
        storagePath: relFilePath,
        fileHash,
      });
    }

    // 更新总时长
    await db.update(schema.audiobook)
      .set({ totalDuration, chapterCount: files.length })
      .where(eq(schema.audiobook.id, book.id));

    logger.info({ title, chapters: files.length, duration: totalDuration }, 'Audiobook scanned');
  }
}

export const audiobookScannerService = new AudiobookScannerService();
