# ── 构建阶段 ──────────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app

# 先复制依赖文件（利用 Docker 缓存层）
COPY package.json package-lock.json* ./
RUN npm ci --ignore-scripts

# 复制源码并编译
COPY tsconfig.json ./
COPY src/ ./src/
RUN npm run build

# 清理 devDependencies
RUN npm prune --omit=dev

# ── 生产阶段 ──────────────────────────────────────────────
FROM node:20-alpine

# 安装 FFmpeg（转码依赖）和 wget（健康检查）
RUN apk add --no-cache ffmpeg wget

WORKDIR /app

# 从构建阶段复制编译产物和依赖
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package.json ./

# 创建数据目录
RUN mkdir -p /app/data /app/data/transcode-cache

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=30s \
  CMD wget --quiet --tries=1 --spider http://localhost:3000/api/health || exit 1

CMD ["node", "dist/index.js"]
