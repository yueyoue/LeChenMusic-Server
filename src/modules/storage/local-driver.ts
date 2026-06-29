import { createReadStream, statSync, existsSync, readdirSync } from 'fs';
import { resolve, join, relative } from 'path';
import type { Readable } from 'stream';
import type { StorageDriver, FileStat, FileItem, StreamRange, StorageType } from './types.js';

export class LocalDriver implements StorageDriver {
  constructor(private basePath: string) {
    this.basePath = resolve(basePath);
  }

  getType(): StorageType {
    return 'local';
  }

  /** 安全路径解析 - 防止路径遍历攻击 */
  private safePath(filePath: string): string {
    const resolved = resolve(this.basePath, filePath);
    if (!resolved.startsWith(this.basePath)) {
      throw new Error('Path traversal detected');
    }
    return resolved;
  }

  async exists(filePath: string): Promise<boolean> {
    try {
      const fullPath = this.safePath(filePath);
      return existsSync(fullPath);
    } catch {
      return false;
    }
  }

  async createReadStream(filePath: string, range?: StreamRange): Promise<Readable> {
    const fullPath = this.safePath(filePath);
    const options: { start?: number; end?: number } = {};
    if (range) {
      options.start = range.start;
      options.end = range.end;
    }
    return createReadStream(fullPath, options);
  }

  async stat(filePath: string): Promise<FileStat> {
    const fullPath = this.safePath(filePath);
    const stat = statSync(fullPath);
    return {
      size: stat.size,
      mtime: stat.mtime,
      isFile: stat.isFile(),
      isDirectory: stat.isDirectory(),
    };
  }

  async listDir(dirPath: string): Promise<FileItem[]> {
    const fullPath = this.safePath(dirPath);
    const entries = readdirSync(fullPath, { withFileTypes: true });
    return entries.map((entry) => {
      const entryPath = join(fullPath, entry.name);
      const relPath = relative(this.basePath, entryPath);
      let size: number | undefined;
      let mtime: Date | undefined;
      try {
        const s = statSync(entryPath);
        size = s.size;
        mtime = s.mtime;
      } catch {
        // ignore
      }
      return {
        name: entry.name,
        path: relPath,
        isFile: entry.isFile(),
        isDirectory: entry.isDirectory(),
        size,
        mtime,
      };
    });
  }
}
