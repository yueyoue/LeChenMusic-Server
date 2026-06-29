import 'dotenv/config';

function env(key: string, fallback?: string): string {
  const val = process.env[key] ?? fallback;
  if (val === undefined) throw new Error(`Missing env: ${key}`);
  return val;
}

export const config = {
  port: parseInt(env('PORT', '3000'), 10),
  host: env('HOST', '0.0.0.0'),
  nodeEnv: env('NODE_ENV', 'development'),
  isDev: env('NODE_ENV', 'development') === 'development',

  db: {
    type: env('DB_TYPE', 'sqlite') as 'sqlite' | 'mysql',
    path: env('DB_PATH', './data/lechen.db'),
  },

  jwt: {
    secret: env('JWT_SECRET', 'dev-secret-change-me'),
    accessExpires: env('JWT_ACCESS_EXPIRES', '2h'),
    refreshExpires: env('JWT_REFRESH_EXPIRES', '30d'),
  },

  transcode: {
    maxTasks: parseInt(env('MAX_TRANSCODE_TASKS', '4'), 10),
    cacheDir: env('TRANSCODE_CACHE_DIR', './data/transcode-cache'),
    cacheMaxSizeMB: parseInt(env('TRANSCODE_CACHE_MAX_SIZE_MB', '2048'), 10),
  },

  log: {
    level: env('LOG_LEVEL', 'info'),
  },

  cors: {
    origins: env('CORS_ORIGINS', '').split(',').filter(Boolean),
  },
};
