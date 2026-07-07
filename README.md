# LeChenMusic Server

基于 [Navidrome](https://github.com/navidrome/navidrome) 的私有音乐+有声书流媒体服务器。

## ✨ 功能

- 🎵 **音乐播放** — 全格式支持（MP3/FLAC/AAC/OGG/WAV/APE/DSD...）
- 📖 **有声书** — 有声读物、评书、相声、戏曲、儿童、教育
- 🎤 **演播者** — 类似音乐的艺术家，按演播者浏览
- ▶️ **章节播放** — 进度同步、书签、独立封面
- ⏪ **±15秒跳转** — 有声书播放器专属
- ⏩ **跳过片头片尾** — 可自定义秒数
- 🏷️ **分类筛选** — 有声读物/评书/相声/戏曲/儿童/教育
- ⭐ **收藏功能** — 独立于音乐的有声书收藏
- 🔍 **搜索** — 按书名、作者、演播者搜索
- 📝 **歌单管理** — 公开/私密歌单
- ❤️ **收藏** — 歌曲/专辑/艺人/有声书
- 📻 **电台** — Internet 电台
- 🔍 **全局搜索** — 歌曲/专辑/艺人/有声书
- 👥 **多用户** — 每用户独立进度、收藏
- 📱 **Subsonic API** — 兼容所有第三方客户端
- 🎨 **Web 管理界面** — React 管理后台
- 🔄 **自动监控** — 媒体库变更自动扫描

## 📦 安装

### 方式一：Docker Compose（推荐）

创建 `docker-compose.yml`：

```yaml
services:
  lechen-music:
    image: ghcr.io/yueyoue/lechenmusic-server:latest
    container_name: lechen-music
    restart: unless-stopped
    ports:
      - "3334:3334"
    environment:
      - TZ=Asia/Shanghai
      - ND_PORT=3334
    volumes:
      - ./data:/data
      - /path/to/music:/music:ro
      - /path/to/audiobooks:/audiobooks:ro
```

启动：

```bash
docker compose up -d
```

打开浏览器访问 `http://你的IP:3334`

### 方式二：Docker 命令行

```bash
docker run -d \
  --name lechen-music \
  -p 3334:3334 \
  -v /vol1/docker/lechen-music/data:/data \
  -v /path/to/music:/music:ro \
  -v /path/to/audiobooks:/audiobooks:ro \
  -e TZ=Asia/Shanghai \
  -e ND_PORT=3334 \
  --restart unless-stopped \
  ghcr.io/yueyoue/lechenmusic-server:latest
```

### 方式三：飞牛 NAS

1. 打开飞牛 Docker 管理
2. 新建 Compose
3. 粘贴上面的 `docker-compose.yml` 内容
4. 修改音乐和有声书路径
5. 启动

## 📁 媒体库配置

启动后在 Web 后台添加媒体库：

### 音乐目录结构

```
/music/
  ├── 周杰伦/
  │   ├── 范特西/
  │   │   ├── 01 - 爱在西元前.flac
  │   │   └── cover.jpg
  │   └── ...
  └── ...
```

### 有声书目录结构

```
/audiobooks/
  ├── 有声读物/          ← 分类目录（可选）
  │   ├── 三体/
  │   │   ├── 01.mp3
  │   │   ├── 02.mp3
  │   │   └── cover.jpg
  │   └── 红楼梦/
  │       ├── 第01回.mp3
  │       └── folder.jpg
  ├── 评书/
  │   └── 三国演义-单田芳/
  │       ├── 001.mp3
  │       └── ...
  └── 相声/
      └── 德云社精选/
          ├── 01.mp3
          └── ...
```

> 💡 音乐和有声书放在**不同目录**，添加媒体库时选择对应类型。
> 
> 💡 分类目录名（有声读物/评书/相声/戏曲/儿童/教育）会被自动识别为 genre。

## ⚙️ 配置

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ND_PORT` | `3334` | 服务端口 |
| `ND_MUSICFOLDER` | `/music` | 默认音乐目录 |
| `ND_DATAFOLDER` | `/data` | 数据目录（数据库、缓存） |

## 🔄 更新版本

### Docker Compose

```bash
docker compose pull
docker compose up -d
```

### Docker 命令行

```bash
docker pull ghcr.io/yueyoue/lechenmusic-server:latest
docker stop lechen-music
docker rm lechen-music
# 重新运行上面的 docker run 命令
```

> 数据保存在 `./data` 目录，更新不会丢失任何数据。

## 📱 配套 APP

- [LeChenMusic-Player](https://github.com/yueyoue/LeChenMusic-Player)（Android）

## 🔧 开发

```bash
git clone https://github.com/yueyoue/LeChenMusic-Server.git
cd LeChenMusic-Server

# 开发模式
make dev

# 编译
make build
```

## 📄 基于

- [Navidrome](https://github.com/navidrome/navidrome) - 开源音乐服务器
- [ting-reader](https://github.com/dqsq2e2/ting-reader) - 有声书平台（UI 参考）
