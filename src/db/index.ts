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
