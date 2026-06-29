import Database from 'better-sqlite3';
import { drizzle } from 'drizzle-orm/better-sqlite3';
import { config } from '../config/index.js';
import * as schema from './schema.js';
import { mkdirSync } from 'fs';
import { dirname } from 'path';

// 确保数据库目录存在
mkdirSync(dirname(config.db.path), { recursive: true });

const sqlite = new Database(config.db.path);
sqlite.pragma('journal_mode = WAL');
sqlite.pragma('foreign_keys = ON');
sqlite.pragma('busy_timeout = 5000');

export const db = drizzle(sqlite, { schema });
export { schema };

// 自动建表
export function initDatabase() {
  sqlite.exec(`
    CREATE TABLE IF NOT EXISTS sys_user (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      username TEXT NOT NULL UNIQUE,
      password_hash TEXT NOT NULL,
      role TEXT DEFAULT 'user' NOT NULL,
      avatar TEXT,
      display_name TEXT,
      created_at INTEGER,
      updated_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS media_library (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT NOT NULL,
      storage_type TEXT NOT NULL,
      storage_path TEXT NOT NULL,
      scan_status TEXT DEFAULT 'idle',
      last_scan_at INTEGER,
      file_count INTEGER DEFAULT 0,
      created_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS artist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT NOT NULL,
      name_sort TEXT,
      avatar_path TEXT,
      bio TEXT,
      created_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS album (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      title TEXT NOT NULL,
      artist_id INTEGER REFERENCES artist(id),
      year INTEGER,
      genre TEXT,
      cover_path TEXT,
      total_discs INTEGER DEFAULT 1,
      total_tracks INTEGER,
      description TEXT,
      created_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS track (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      title TEXT NOT NULL,
      artist_id INTEGER REFERENCES artist(id),
      album_id INTEGER REFERENCES album(id),
      duration INTEGER,
      bitrate INTEGER,
      bit_depth INTEGER,
      sample_rate INTEGER,
      format TEXT,
      file_size INTEGER,
      track_number INTEGER,
      disc_number INTEGER DEFAULT 1,
      genre TEXT,
      storage_type TEXT NOT NULL,
      storage_path TEXT NOT NULL,
      file_hash TEXT,
      created_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS lyric (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      track_id INTEGER REFERENCES track(id) ON DELETE CASCADE,
      language TEXT DEFAULT 'original',
      type TEXT DEFAULT 'lrc',
      content TEXT NOT NULL,
      is_synced INTEGER DEFAULT 0,
      created_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS playlist (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id INTEGER REFERENCES sys_user(id) ON DELETE CASCADE,
      name TEXT NOT NULL,
      cover_path TEXT,
      description TEXT,
      created_at INTEGER,
      updated_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS playlist_track (
      playlist_id INTEGER REFERENCES playlist(id) ON DELETE CASCADE,
      track_id INTEGER REFERENCES track(id) ON DELETE CASCADE,
      sort_order INTEGER DEFAULT 0,
      added_at INTEGER,
      PRIMARY KEY (playlist_id, track_id)
    );

    CREATE TABLE IF NOT EXISTS favorite (
      user_id INTEGER REFERENCES sys_user(id) ON DELETE CASCADE,
      target_type TEXT NOT NULL,
      target_id INTEGER NOT NULL,
      created_at INTEGER,
      PRIMARY KEY (user_id, target_type, target_id)
    );

    CREATE TABLE IF NOT EXISTS play_history (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      user_id INTEGER REFERENCES sys_user(id),
      track_id INTEGER REFERENCES track(id),
      position INTEGER DEFAULT 0,
      duration INTEGER,
      played_at INTEGER
    );

    CREATE TABLE IF NOT EXISTS transcode_cache (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      source_hash TEXT NOT NULL,
      target_bitrate INTEGER NOT NULL,
      target_format TEXT NOT NULL,
      cache_path TEXT NOT NULL,
      file_size INTEGER,
      created_at INTEGER,
      last_accessed INTEGER,
      access_count INTEGER DEFAULT 0
    );

    CREATE TABLE IF NOT EXISTS sys_config (
      key TEXT PRIMARY KEY,
      value TEXT,
      updated_at INTEGER
    );
  `);
}
