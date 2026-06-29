import { sqliteTable, text, integer, real } from 'drizzle-orm/sqlite-core';

// ─── 用户表 ───────────────────────────────────────────────
export const sysUser = sqliteTable('sys_user', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  username: text('username').notNull().unique(),
  passwordHash: text('password_hash').notNull(),
  role: text('role', { enum: ['admin', 'user'] }).default('user').notNull(),
  avatar: text('avatar'),
  displayName: text('display_name'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
  updatedAt: integer('updated_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 媒体库表 ─────────────────────────────────────────────
export const mediaLibrary = sqliteTable('media_library', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  name: text('name').notNull(),
  storageType: text('storage_type', { enum: ['local', 'smb', 'nfs'] }).notNull(),
  storagePath: text('storage_path').notNull(),
  scanStatus: text('scan_status', { enum: ['idle', 'scanning', 'error'] }).default('idle'),
  lastScanAt: integer('last_scan_at', { mode: 'timestamp' }),
  fileCount: integer('file_count').default(0),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 艺人表 ───────────────────────────────────────────────
export const artist = sqliteTable('artist', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  name: text('name').notNull(),
  nameSort: text('name_sort'),
  avatarPath: text('avatar_path'),
  bio: text('bio'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 专辑表 ───────────────────────────────────────────────
export const album = sqliteTable('album', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  title: text('title').notNull(),
  artistId: integer('artist_id').references(() => artist.id),
  year: integer('year'),
  genre: text('genre'),
  coverPath: text('cover_path'),
  totalDiscs: integer('total_discs').default(1),
  totalTracks: integer('total_tracks'),
  description: text('description'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 音轨表 ───────────────────────────────────────────────
export const track = sqliteTable('track', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  title: text('title').notNull(),
  artistId: integer('artist_id').references(() => artist.id),
  albumId: integer('album_id').references(() => album.id),
  duration: integer('duration'),          // 秒
  bitrate: integer('bitrate'),            // kbps
  bitDepth: integer('bit_depth'),         // 16/24
  sampleRate: integer('sample_rate'),     // 44100/48000/96000
  format: text('format'),                 // mp3/flac/aac/ogg/wav/ape/dsf
  fileSize: integer('file_size'),         // 字节
  trackNumber: integer('track_number'),
  discNumber: integer('disc_number').default(1),
  genre: text('genre'),
  storageType: text('storage_type', { enum: ['local', 'smb', 'nfs'] }).notNull(),
  storagePath: text('storage_path').notNull(),
  fileHash: text('file_hash'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 歌词表 ───────────────────────────────────────────────
export const lyric = sqliteTable('lyric', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  trackId: integer('track_id').references(() => track.id, { onDelete: 'cascade' }),
  language: text('language').default('original'),    // original/translation/romanized
  type: text('type').default('lrc'),                 // lrc/txt/srt
  content: text('content').notNull(),
  isSynced: integer('is_synced', { mode: 'boolean' }).default(false),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 歌单表 ───────────────────────────────────────────────
export const playlist = sqliteTable('playlist', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => sysUser.id, { onDelete: 'cascade' }),
  name: text('name').notNull(),
  coverPath: text('cover_path'),
  description: text('description'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
  updatedAt: integer('updated_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 歌单歌曲关联表 ──────────────────────────────────────
export const playlistTrack = sqliteTable('playlist_track', {
  playlistId: integer('playlist_id').references(() => playlist.id, { onDelete: 'cascade' }),
  trackId: integer('track_id').references(() => track.id, { onDelete: 'cascade' }),
  sortOrder: integer('sort_order').default(0),
  addedAt: integer('added_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 收藏表 ───────────────────────────────────────────────
export const favorite = sqliteTable('favorite', {
  userId: integer('user_id').references(() => sysUser.id, { onDelete: 'cascade' }),
  targetType: text('target_type', { enum: ['track', 'album', 'artist'] }).notNull(),
  targetId: integer('target_id').notNull(),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 播放历史表 ───────────────────────────────────────────
export const playHistory = sqliteTable('play_history', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => sysUser.id),
  trackId: integer('track_id').references(() => track.id),
  position: integer('position').default(0),     // 播放进度（秒）
  duration: integer('duration'),                 // 播放时长（秒）
  playedAt: integer('played_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 转码缓存表 ───────────────────────────────────────────
export const transcodeCache = sqliteTable('transcode_cache', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  sourceHash: text('source_hash').notNull(),
  targetBitrate: integer('target_bitrate').notNull(),
  targetFormat: text('target_format').notNull(),
  cachePath: text('cache_path').notNull(),
  fileSize: integer('file_size'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
  lastAccessed: integer('last_accessed', { mode: 'timestamp' }).$defaultFn(() => new Date()),
  accessCount: integer('access_count').default(0),
});

// ─── 系统配置表 ───────────────────────────────────────────
export const sysConfig = sqliteTable('sys_config', {
  key: text('key').primaryKey(),
  value: text('value'),
  updatedAt: integer('updated_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});
