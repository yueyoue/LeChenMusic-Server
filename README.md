# LeChenMusic Server

私有部署的音乐流媒体服务端，替代 Navidrome，配合 LeChenMusic Android 客户端使用。

## 特性

- 🎵 音频文件扫描与元数据自动解析
- 🔄 实时音频转码（FFmpeg）
- 💾 混合存储支持（本地磁盘 / SMB / NFS / rclone 云盘挂载）
- 🔐 JWT 鉴权 + 角色权限管理
- 📱 Subsonic API 兼容（支持第三方客户端）
- 🖥️ Web 管理后台
- 🐳 Docker 一键部署（x86_64 / ARM64）

## 快速开始

### Docker 部署（推荐）

```bash
# 克隆项目
git clone https://github.com/yueyoue/LeChenMusic-Server.git
cd LeChenMusic-Server

# 修改配置
cp .env.example .env
# 编辑 .env 修改 JWT_SECRET

# 修改 docker-compose.yml 中的音乐目录路径
# /path/to/music → 你的实际音乐目录

# 启动
docker-compose up -d
```

服务启动后：
- API 服务：`http://localhost:3000`
- 管理后台：`http://localhost:3000/admin`
- 健康检查：`http://localhost:3000/api/health`

### 本地开发

```bash
# 安装依赖
npm install

# 复制配置
cp .env.example .env

# 启动开发服务器（热重载）
npm run dev

# 数据库迁移
npm run db:generate
npm run db:migrate
```

## 项目结构

```
src/
├── config/           # 配置管理
├── db/               # 数据库 schema 与连接
├── middleware/        # Express 中间件（鉴权、错误处理）
├── modules/
│   ├── admin/        # 后台管理 API
│   ├── album/        # 专辑模块
│   ├── artist/       # 艺人模块
│   ├── auth/         # 认证模块（登录/注册/Token 刷新）
│   ├── favorite/     # 收藏模块
│   ├── history/      # 播放历史
│   ├── library/      # 媒体库管理与扫描
│   ├── playlist/     # 歌单模块
│   ├── search/       # 全局搜索
│   ├── storage/      # 存储驱动抽象层
│   └── track/        # 音轨模块（列表/播放/转码）
└── utils/            # 工具函数（日志等）
```

## API 文档

启动服务后访问 `/api-docs` 查看 Swagger 文档。

### 主要接口

| 分组 | 说明 |
|------|------|
| `POST /api/auth/login` | 用户登录 |
| `POST /api/auth/register` | 用户注册 |
| `GET /api/tracks` | 音轨列表 |
| `GET /api/tracks/:id/stream` | 流式播放 |
| `GET /api/search?q=xxx` | 全局搜索 |
| `GET /api/playlists` | 歌单列表 |
| `GET /api/favorites` | 收藏列表 |
| `GET /api/history` | 播放历史 |
| `GET /api/admin/dashboard` | 仪表盘统计 |

## 音乐目录挂载

### 本地目录
直接修改 `docker-compose.yml` 中的 volume 映射：
```yaml
volumes:
  - /your/music/path:/music:ro
```

### NAS SMB 挂载
在宿主机挂载 SMB 后映射到容器：
```bash
# 宿主机挂载
mount -t cifs //nas/music /mnt/nas-music -o username=xxx,password=xxx

# docker-compose.yml
volumes:
  - /mnt/nas-music:/music:ro
```

### 云盘（rclone）
```bash
# 宿主机安装 rclone 并挂载云盘
rclone mount aliyun:/music /mnt/cloud-music --vfs-cache-mode full

# docker-compose.yml
volumes:
  - /mnt/cloud-music:/music:ro
```

## 技术栈

- **运行时**：Node.js 20 LTS + TypeScript
- **框架**：Express 5
- **数据库**：SQLite（Drizzle ORM）
- **音频处理**：FFmpeg + music-metadata
- **鉴权**：JWT + argon2id
- **容器化**：Docker (Alpine)

## License

MIT
