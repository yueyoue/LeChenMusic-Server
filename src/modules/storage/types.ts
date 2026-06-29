import type { Readable } from 'stream';

export interface FileStat {
  size: number;
  mtime: Date;
  isFile: boolean;
  isDirectory: boolean;
}

export interface FileItem {
  name: string;
  path: string;
  isFile: boolean;
  isDirectory: boolean;
  size?: number;
  mtime?: Date;
}

export interface StreamRange {
  start: number;
  end: number;
}

export type StorageType = 'local' | 'smb' | 'nfs';

export interface StorageDriver {
  exists(path: string): Promise<boolean>;
  createReadStream(path: string, range?: StreamRange): Promise<Readable>;
  stat(path: string): Promise<FileStat>;
  listDir(path: string): Promise<FileItem[]>;
  getType(): StorageType;
}
