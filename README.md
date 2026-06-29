# LeChenMusic Server

私有部署音乐流媒体服务端，配合 LeChenMusic Android 客户端使用。

## 🚀 飞牛 NAS 一键部署

### 第一步：创建 docker-compose.yml

在飞牛 NAS 的 Docker 目录下创建 `docker-compose.yml`，粘贴以下内容：

```yaml
version: '3.8'
services:
  lechen-music:
    image: ghcr.io/yueyoue/lechenmusic-server:latest
    container_name: lechen-music
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - TZ=Asia/Shanghai
      - NODE_ENV=production
      - JWT_SECRET=改成你自己的密钥
      - MAX_TRANSCODE_TASKS=4
      - LOG_LEVEL=info
    volumes:
      - ./data:/app/data
      - /vol1/music:/music:ro    # ← 改成你的音乐目录
```

### 第二步：启动

在飞牛 Docker 管理界面中，选择「Compose」→「导入」→ 选择这个 yml 文件 → 点击「启动」

首次启动会自动拉取镜像，等待几分钟即可。

### 第三步：使用

- 管理后台：`http://你的NAS-IP:3000`
- 首个注册的用户自动成为管理员
- 在后台「媒体库管理」中添加音乐目录（容器内路径为 `/music`）

## 📱 客户端连接

在 LeChenMusic APP 的设置中：
- 服务端地址：`http://你的NAS-IP:3000`
- 用户名/密码：后台注册的账号

## 🔧 配置说明

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `JWT_SECRET` | Token 加密密钥，改成任意字符串 | 必须修改 |
| `MAX_TRANSCODE_TASKS` | 最大同时转码数 | 4 |
| `LOG_LEVEL` | 日志级别 (info/debug/warn/error) | info |
| `TZ` | 时区 | Asia/Shanghai |

## 📂 音乐目录挂载

### 飞牛 NAS 本地目录
```yaml
volumes:
  - /vol1/music:/music:ro
```

### SMB 网络挂载
先在飞牛系统中挂载 SMB 共享，然后映射挂载路径：
```yaml
volumes:
  - /mnt/smb/music:/music:ro
```

### 云盘（通过 rclone）
```bash
# 先在 NAS 上安装 rclone 并挂载
rclone mount aliyun:/music /mnt/cloud-music --vfs-cache-mode full
```
```yaml
volumes:
  - /mnt/cloud-music:/music:ro
```

## 🏗️ 技术栈

- Node.js 20 + TypeScript + Express 5
- SQLite + Drizzle ORM
- FFmpeg 实时转码
- JWT 鉴权

## 📖 API

启动后访问 `http://NAS-IP:3000/api/health` 检查服务状态。

主要接口：
- `POST /api/auth/login` — 登录
- `POST /api/auth/register` — 注册
- `GET /api/tracks` — 歌曲列表
- `GET /api/tracks/:id/stream` — 播放
- `GET /api/search?q=xxx` — 搜索

## License

MIT
