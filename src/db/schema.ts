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
  mediaType: text('media_type', { enum: ['music', 'audiobook', 'video'] }).default('music').notNull(),
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
  duration: integer('duration'),
  bitrate: integer('bitrate'),
  bitDepth: integer('bit_depth'),
  sampleRate: integer('sample_rate'),
  format: text('format'),
  fileSize: integer('file_size'),
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
  language: text('language').default('original'),
  type: text('type').default('lrc'),
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
  targetType: text('target_type', { enum: ['track', 'album', 'artist', 'audiobook'] }).notNull(),
  targetId: integer('target_id').notNull(),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 播放历史表 ───────────────────────────────────────────
export const playHistory = sqliteTable('play_history', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => sysUser.id),
  trackId: integer('track_id').references(() => track.id),
  position: integer('position').default(0),
  duration: integer('duration'),
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

// ═══════════════════════════════════════════════════════════
// 有声书模块
// ═══════════════════════════════════════════════════════════

// ─── 有声书表 ─────────────────────────────────────────────
export const audiobook = sqliteTable('audiobook', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  title: text('title').notNull(),
  author: text('author'),
  narrator: text('narrator'),               // 演播者
  coverPath: text('cover_path'),
  description: text('description'),
  genre: text('genre'),                      // 分类：小说/历史/经管/教育...
  year: integer('year'),
  totalDuration: integer('total_duration'),  // 总时长（秒）
  chapterCount: integer('chapter_count'),    // 章节数
  storageType: text('storage_type', { enum: ['local', 'smb', 'nfs'] }).notNull(),
  storagePath: text('storage_path').notNull(), // 有声书根目录
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 有声书章节表 ─────────────────────────────────────────
export const audiobookChapter = sqliteTable('audiobook_chapter', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  audiobookId: integer('audiobook_id').references(() => audiobook.id, { onDelete: 'cascade' }),
  title: text('title').notNull(),
  chapterNumber: integer('chapter_number').notNull(),
  duration: integer('duration'),             // 时长（秒）
  format: text('format'),
  fileSize: integer('file_size'),
  storagePath: text('storage_path').notNull(), // 相对于有声书目录
  fileHash: text('file_hash'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 有声书播放进度表 ─────────────────────────────────────
export const audiobookProgress = sqliteTable('audiobook_progress', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => sysUser.id, { onDelete: 'cascade' }),
  audiobookId: integer('audiobook_id').references(() => audiobook.id, { onDelete: 'cascade' }),
  chapterId: integer('chapter_id').references(() => audiobookChapter.id),
  chapterNumber: integer('chapter_number'),  // 当前章节序号
  position: integer('position').default(0),  // 章节内播放位置（秒）
  completed: integer('completed', { mode: 'boolean' }).default(false), // 是否听完
  lastPlayedAt: integer('last_played_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});

// ─── 有声书书签表 ─────────────────────────────────────────
export const audiobookBookmark = sqliteTable('audiobook_bookmark', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => sysUser.id, { onDelete: 'cascade' }),
  audiobookId: integer('audiobook_id').references(() => audiobook.id, { onDelete: 'cascade' }),
  chapterId: integer('chapter_id').references(() => audiobookChapter.id),
  position: integer('position').notNull(),   // 书签位置（秒）
  title: text('title'),                      // 书签备注
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
});
