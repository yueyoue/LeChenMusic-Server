import { spawn, type ChildProcess } from 'child_process';
import { createReadStream, existsSync, mkdirSync, statSync, unlinkSync } from 'fs';
import { join, extname } from 'path';
import { createHash } from 'crypto';
import { db, schema } from '../../db/index.js';
import { eq, and, sql } from 'drizzle-orm';
import { config } from '../../config/index.js';
import { logger } from '../../utils/logger.js';

interface TranscodeJob {
  id: string;
  process: ChildProcess;
  sourcePath: string;
  targetBitrate: number;
  targetFormat: string;
  cachePath: string;
  startTime: number;
  aborted: boolean;
}

export class TranscodeService {
  private jobs = new Map<string, TranscodeJob>();
  private queue: Array<() => Promise<void>> = [];
  private running = 0;

  constructor() {
    mkdirSync(config.transcode.cacheDir, { recursive: true });
  }

  /** 获取转码流（优先缓存，否则实时转码） */
  async getTranscodeStream(
    sourcePath: string,
    targetBitrate: number,
    targetFormat: string = 'mp3',
  ): Promise<{ stream: NodeJS.ReadableStream; fromCache: boolean }> {
    const sourceHash = this.hashFile(sourcePath);
    const cacheKey = `${sourceHash}_${targetBitrate}_${targetFormat}`;
    const cachePath = join(config.transcode.cacheDir, `${cacheKey}.${targetFormat}`);

    // 检查缓存
    if (existsSync(cachePath)) {
      this.updateCacheAccess(sourceHash, targetBitrate, targetFormat, cachePath);
      logger.info({ cacheKey }, 'Transcode cache hit');
      return { stream: createReadStream(cachePath), fromCache: true };
    }

    // 检查并发限制
    if (this.running >= config.transcode.maxTasks) {
      // 排队或降级直链
      logger.warn('Transcode queue full, falling back to direct stream');
      return { stream: createReadStream(sourcePath), fromCache: false };
    }

    // 实时转码
    const stream = await this.startTranscode(sourcePath, cachePath, targetBitrate, targetFormat);
    return { stream, fromCache: false };
  }

  private async startTranscode(
    sourcePath: string,
    cachePath: string,
    targetBitrate: number,
    targetFormat: string,
  ): Promise<NodeJS.ReadableStream> {
    const jobId = `tc_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

    const args = [
      '-i', sourcePath,
      '-ab', `${targetBitrate}k`,
      '-f', targetFormat,
      '-map_metadata', '-1',
      ...(targetFormat === 'mp3' ? ['-codec:a', 'libmp3lame'] : []),
      ...(targetFormat === 'aac' ? ['-codec:a', 'aac'] : []),
      'pipe:1',
    ];

    const ffmpeg = spawn('ffmpeg', args, { stdio: ['pipe', 'pipe', 'pipe'] });

    const job: TranscodeJob = {
      id: jobId,
      process: ffmpeg,
      sourcePath,
      targetBitrate,
      targetFormat,
      cachePath,
      startTime: Date.now(),
      aborted: false,
    };

    this.jobs.set(jobId, job);
    this.running++;

    // 同时写入缓存文件
    const { createWriteStream } = await import('fs');
    const cacheStream = createWriteStream(cachePath);

    ffmpeg.stdout.on('data', (chunk) => {
      cacheStream.write(chunk);
    });

    ffmpeg.on('close', () => {
      cacheStream.end();
      this.running--;
      this.jobs.delete(jobId);

      if (job.aborted) {
        try { unlinkSync(cachePath); } catch {}
      } else {
        // 记录缓存
        this.saveCacheRecord(sourcePath, targetBitrate, targetFormat, cachePath);
        logger.info({ jobId, duration: Date.now() - job.startTime }, 'Transcode completed');
      }
    });

    ffmpeg.on('error', (err) => {
      logger.error({ jobId, err: err.message }, 'Transcode error');
      this.running--;
      this.jobs.delete(jobId);
      try { unlinkSync(cachePath); } catch {}
    });

    return ffmpeg.stdout;
  }

  /** 中止转码（客户端断开时调用） */
  abort(jobId: string) {
    const job = this.jobs.get(jobId);
    if (job) {
      job.aborted = true;
      job.process.kill('SIGKILL');
    }
  }

  /** 中止所有转码 */
  abortAll() {
    for (const [id, job] of this.jobs) {
      job.aborted = true;
      job.process.kill('SIGKILL');
    }
  }

  private hashFile(path: string): string {
    try {
      const stat = statSync(path);
      return createHash('md5').update(`${path}:${stat.size}:${stat.mtimeMs}`).digest('hex').slice(0, 16);
    } catch {
      return createHash('md5').update(path).digest('hex').slice(0, 16);
    }
  }

  private async saveCacheRecord(sourcePath: string, bitrate: number, format: string, cachePath: string) {
    const sourceHash = this.hashFile(sourcePath);
    try {
      const stat = statSync(cachePath);
      await db.insert(schema.transcodeCache).values({
        sourceHash, targetBitrate: bitrate, targetFormat: format,
        cachePath, fileSize: stat.size,
      }).onConflictDoNothing();
    } catch {}
  }

  private async updateCacheAccess(sourceHash: string, bitrate: number, format: string, cachePath: string) {
    try {
      await db.update(schema.transcodeCache)
        .set({ lastAccessed: new Date(), accessCount: sql`access_count + 1` })
        .where(and(
          eq(schema.transcodeCache.sourceHash, sourceHash),
          eq(schema.transcodeCache.targetBitrate, bitrate),
          eq(schema.transcodeCache.targetFormat, format),
        ));
    } catch {}
  }

  /** 清理旧缓存（LRU） */
  async cleanupCache(maxSizeMB: number) {
    try {
      const totalSize = await db.select({ sum: sql<number>`coalesce(sum(file_size), 0)` })
        .from(schema.transcodeCache).get();

      if ((totalSize?.sum ?? 0) > maxSizeMB * 1024 * 1024) {
        // 删除最旧的缓存
        const old = await db.select().from(schema.transcodeCache)
          .orderBy(schema.transcodeCache.lastAccessed)
          .limit(10).all();

        for (const item of old) {
          try { unlinkSync(item.cachePath); } catch {}
          await db.delete(schema.transcodeCache).where(eq(schema.transcodeCache.id, item.id));
        }

        logger.info({ deleted: old.length }, 'Transcode cache cleanup');
      }
    } catch {}
  }
}

export const transcodeService = new TranscodeService();
