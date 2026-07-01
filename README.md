# LeChenMusic Server

私有部署音乐流媒体服务端，配合 LeChenMusic Android 客户端使用。

## 🚀 飞牛 NAS 一键部署

### SSH 方式（推荐）

用 SSH 登录飞牛 NAS，依次执行以下命令：

```bash
# 1. 下载项目
cd /vol1/1000
git clone https://github.com/yueyoue/LeChenMusic-Server.git
cd LeChenMusic-Server

# 2. 启动（首次会自动构建，约2-3分钟）
docker compose up -d --build
```

部署完成！访问 `http://你的NAS-IP:3000` 即可使用。

> **修改音乐目录：** 编辑 `docker-compose.yml` 中的 volumes，把 `/vol1/1000/音乐` 改成你的实际路径，然后执行 `docker compose up -d --build` 重建。

### 飞牛界面方式

1. 打开飞牛 **Docker** → **Compose** → **新建项目**
2. 项目名称填 `lechen-music`
3. 选择「上传」，上传项目里的 `docker-compose.yml` 文件
4. 点击「启动」，等待构建完成

## 📱 使用

- **管理后台：** `http://你的NAS-IP:3000`
- **首个注册的用户自动成为管理员**
- 进入后台 → **媒体库** → **添加媒体路径** → 选择音乐目录 → 点击 **扫描**

## 📱 客户端连接

在 LeChenMusic APP 的设置中：
- 服务端地址：`http://你的NAS-IP:3000`
- 用户名/密码：后台注册的账号

## 🔧 配置说明

编辑 `docker-compose.yml` 中的环境变量：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `JWT_SECRET` | Token 加密密钥，改成任意字符串 | 需要修改 |
| `MAX_TRANSCODE_TASKS` | 最大同时转码数 | 4 |
| `LOG_LEVEL` | 日级 (info/debug/warn/error) | info |
| `TZ` | 时区 | Asia/Shanghai |

修改后执行 `docker compose up -d` 重启生效。

## 📂 音乐目录配置

`docker-compose.yml` 中的 volumes 配置音乐目录：

```yaml
volumes:
  - ./data:/app/data           # 数据库（不要改）
  - /vol1/1000/音乐:/music:ro  # ← 改成你的音乐目录
```

### 常见路径示例

```yaml
# 飞牛默认音乐目录
- /vol1/1000/音乐:/music:ro

# 自定义目录
- /vol2/data/my-music:/music:ro

# SMB 网络挂载（先在飞牛系统中挂载）
- /mnt/smb/music:/music:ro
```

修改后执行 `docker compose up -d --build` 重建生效。

## 🏗️ 技术栈

- Node.js 20 + TypeScript + Express 5
- SQLite + Drizzle ORM
- music-metadata 音频元数据解析
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
