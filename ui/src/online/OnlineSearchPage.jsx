import React, { useState, useEffect, useCallback, useRef } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  Tooltip, Chip, TextField, Switch, FormControlLabel, CircularProgress,
  MenuItem, LinearProgress,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import RefreshIcon from '@material-ui/icons/Refresh'
import GetAppIcon from '@material-ui/icons/GetApp'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import { Alert } from '@material-ui/lab'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「在线搜歌」页（M5）：搜索全网曲库 → 选择音质下载 → 自动刮削入库。
// 任务面板轮询 /api/online/tasks（3s），完成后提示"已入库，可播放"。

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1100, margin: '0 auto' },
  header: { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 },
  formCard: { marginBottom: 16 },
  formRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' },
  resultCard: { marginBottom: 8 },
  resultRow: { display: 'flex', alignItems: 'center', gap: 12 },
  cover: { width: 56, height: 56, borderRadius: 4, objectFit: 'cover', background: '#222' },
  resultMain: { flex: 1, minWidth: 0 },
  songName: { fontWeight: 600, fontSize: 15 },
  sub: { fontSize: 12, color: theme.palette.text.secondary },
  qualityChip: { marginRight: 6, marginBottom: 4 },
  inLibrary: { opacity: 0.55 },
  taskCard: { marginBottom: 8, borderLeft: '4px solid #1e90ff' },
  taskDone: { borderLeftColor: '#2ed573' },
  taskFailed: { borderLeftColor: '#ff4757' },
  taskRow: { display: 'flex', alignItems: 'center', gap: 8 },
  taskMain: { flex: 1, minWidth: 0 },
  sectionTitle: { fontWeight: 700, marginTop: 24, marginBottom: 12, fontSize: 16 },
  empty: { textAlign: 'center', padding: 48, color: theme.palette.text.secondary },
  progressRow: { display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 },
}))

const QUALITY_OPTIONS = [
  { value: '', label: '默认（无损优先）' },
  { value: 'flac/2000', label: 'FLAC 2000k' },
  { value: 'mp3/320', label: 'MP3 320k' },
  { value: 'mp3/128', label: 'MP3 128k' },
]

const ACTIVE_STATUSES = ['queued', 'resolving', 'downloading', 'tagging']

const fmtDuration = (s) => {
  if (!s || s <= 0) return ''
  const m = Math.floor(s / 60)
  const ss = String(s % 60).padStart(2, '0')
  return `${m}:${ss}`
}

const fmtSize = (bytes) => {
  if (!bytes || bytes <= 0) return ''
  const mb = bytes / 1024 / 1024
  return mb >= 1024 ? `${(mb / 1024).toFixed(2)} GB` : `${mb.toFixed(1)} MB`
}

const qualityLabel = (q) => {
  const fmt = (q.format || '').toUpperCase()
  const parts = [fmt, q.bitrate ? `${q.bitrate}k` : ''].filter(Boolean)
  const head = parts.join(' ')
  return q.size ? `${head} ${q.size}` : head
}

const statusLabel = (t) => {
  switch (t.status) {
    case 'queued': return '排队中'
    case 'resolving': return '解析直链'
    case 'downloading': return '下载中'
    case 'tagging': return '刮削入库'
    case 'done': return '已入库'
    case 'failed': return '失败'
    default: return t.status
  }
}

const progressPct = (t) => {
  if (!t.totalBytes || t.totalBytes <= 0) return 0
  return Math.min(100, Math.round((t.progressBytes / t.totalBytes) * 100))
}

const OnlineSearchPage = () => {
  const classes = useStyles()
  const [name, setName] = useState('')
  const [artist, setArtist] = useState('')
  const [fuzzy, setFuzzy] = useState(false)
  const [quality, setQuality] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState([])
  const [total, setTotal] = useState(0)
  const [searched, setSearched] = useState(false)
  const [tasks, setTasks] = useState([])
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')
  const doneIds = useRef(new Set())
  const firstPoll = useRef(true)

  const fetchTasks = useCallback(async () => {
    try {
      const res = await httpClient(`${REST_URL}/online/tasks?mine=1`)
      const list = res.json || []
      setTasks(list)
      // The first poll only seeds the "already announced" set: tasks finished in
      // an earlier session must not pop a toast when the page opens.
      const announce = !firstPoll.current
      firstPoll.current = false
      list.forEach((t) => {
        if (t.status !== 'done' || doneIds.current.has(t.id)) return
        doneIds.current.add(t.id)
        if (announce) setToast(`《${t.songName}》已入库，可播放`)
      })
    } catch (e) {
      // task panel is best-effort; search errors are reported separately
    }
  }, [])

  useEffect(() => {
    fetchTasks()
    const timer = setInterval(fetchTasks, 3000)
    return () => clearInterval(timer)
  }, [fetchTasks])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 4000)
    return () => clearTimeout(timer)
  }, [toast])

  const handleSearch = async () => {
    const keyword = [name, artist].map((s) => s.trim()).filter(Boolean).join(' ')
    if (!keyword) return
    setSearching(true)
    setError('')
    setSearched(true)
    try {
      const res = await httpClient(
        `${REST_URL}/online/search?keyword=${encodeURIComponent(keyword)}&page=1&size=20`,
      )
      setResults(res.json?.songs || [])
      setTotal(res.json?.total || 0)
    } catch (e) {
      setError('搜索失败：' + (e.message || '请稍后重试'))
      setResults([])
      setTotal(0)
    } finally {
      setSearching(false)
    }
  }

  const handleDownload = async (song) => {
    setError('')
    try {
      await httpClient(`${REST_URL}/online/download`, {
        method: 'POST',
        body: JSON.stringify({
          source: song.source,
          platform: song.platform,
          songId: song.id,
          name: song.name,
          artist: song.artist,
          album: song.albumName,
          coverUrl: song.coverUrl,
          quality,
          fuzzy,
          time: song.time,
          sign: song.sign,
        }),
      })
      setToast(`《${song.name}》已加入下载队列`)
      fetchTasks()
    } catch (e) {
      setError('下载失败：' + (e.message || ''))
    }
  }

  const handleRetry = async (task) => {
    try {
      await httpClient(`${REST_URL}/online/tasks/${task.id}/retry`, { method: 'POST' })
      fetchTasks()
    } catch (e) {
      setError('重试失败：' + (e.message || ''))
    }
  }

  const handleDeleteTask = async (task) => {
    try {
      await httpClient(`${REST_URL}/online/tasks/${task.id}`, { method: 'DELETE' })
      fetchTasks()
    } catch (e) {
      setError('删除失败：' + (e.message || ''))
    }
  }

  const activeTasks = tasks.filter((t) => ACTIVE_STATUSES.includes(t.status))
  const finishedTasks = tasks.filter((t) => t.status === 'done')
  const failedTasks = tasks.filter((t) => t.status === 'failed')

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <MusicNoteIcon style={{ fontSize: 28, color: '#1e90ff' }} />
        <Typography variant="h6" style={{ fontWeight: 700 }}>在线搜歌</Typography>
        <Chip label={`共 ${total} 条结果`} size="small" variant="outlined" />
      </Box>

      {toast && <Alert severity="success" style={{ marginBottom: 12 }}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }}>{error}</Alert>}

      <Card className={classes.formCard} elevation={1}>
        <CardContent>
          <Box className={classes.formRow}>
            <TextField label="歌名" size="small" variant="outlined" value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()} />
            <TextField label="歌手" size="small" variant="outlined" value={artist}
              onChange={(e) => setArtist(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()} />
            <TextField select label="音质" size="small" variant="outlined" value={quality}
              onChange={(e) => setQuality(e.target.value)} style={{ minWidth: 160 }}>
              {QUALITY_OPTIONS.map((o) => (
                <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
              ))}
            </TextField>
            <FormControlLabel
              control={<Switch checked={fuzzy} onChange={(e) => setFuzzy(e.target.checked)} color="primary" />}
              label="模糊匹配（允许下载其他版本）"
            />
            <Button variant="contained" color="primary" startIcon={<SearchIcon />}
              onClick={handleSearch} disabled={searching}>
              {searching ? '搜索中…' : '搜索'}
            </Button>
          </Box>
        </CardContent>
      </Card>

      <Typography className={classes.sectionTitle}>搜索结果</Typography>
      {!searched ? (
        <Box className={classes.empty}>
          <SearchIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 12 }}>输入歌名或歌手开始搜索</Typography>
        </Box>
      ) : searching && results.length === 0 ? (
        <LinearProgress />
      ) : results.length === 0 ? (
        <Box className={classes.empty}>
          <Typography>没有找到匹配的歌曲，试试「模糊匹配」或换个关键词</Typography>
        </Box>
      ) : (
        results.map((song) => (
          <Card className={classes.resultCard} elevation={0} key={`${song.source}-${song.id}`}>
            <CardContent style={{ paddingBottom: 12 }}>
              <Box className={classes.resultRow}>
                {song.coverUrl ? (
                  <img className={classes.cover} src={song.coverUrl} alt="" />
                ) : (
                  <Box className={classes.cover} style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <MusicNoteIcon style={{ color: '#666' }} />
                  </Box>
                )}
                <Box className={[classes.resultMain, song.inLibrary ? classes.inLibrary : ''].join(' ')}>
                  <Typography className={classes.songName} noWrap>{song.name}</Typography>
                  <Typography className={classes.sub} noWrap>
                    {song.artist}{song.albumName ? ` · ${song.albumName}` : ''}
                  </Typography>
                  <Box style={{ marginTop: 4 }}>
                    <Chip size="small" label={song.platform || song.source} className={classes.qualityChip} />
                    {song.duration > 0 && (
                      <Chip size="small" variant="outlined" label={fmtDuration(song.duration)}
                        className={classes.qualityChip} />
                    )}
                    {(song.qualities || []).slice(0, 4).map((q) => (
                      <Chip key={`${q.format}-${q.bitrate}-${q.level || ''}`} size="small" variant="outlined"
                        label={qualityLabel(q)} className={classes.qualityChip} />
                    ))}
                    {song.inLibrary && (
                      <Chip size="small" color="primary" label="已在曲库" className={classes.qualityChip} />
                    )}
                  </Box>
                </Box>
                <Tooltip title={song.inLibrary && !fuzzy ? '曲库已存在，需开启模糊匹配' : '加入下载队列'}>
                  <span>
                    <Button variant="contained" size="small" color="primary" startIcon={<GetAppIcon />}
                      disabled={searching}
                      onClick={() => handleDownload(song)}>下载</Button>
                  </span>
                </Tooltip>
              </Box>
            </CardContent>
          </Card>
        ))
      )}

      <Typography className={classes.sectionTitle}>
        下载任务
        <Button size="small" startIcon={<RefreshIcon />} onClick={fetchTasks} style={{ marginLeft: 8 }}>
          刷新
        </Button>
      </Typography>

      {tasks.length === 0 ? (
        <Box className={classes.empty}>
          <Typography>暂无下载任务</Typography>
        </Box>
      ) : (
        <>
          {activeTasks.map((t) => (
            <Card key={t.id} className={classes.taskCard} elevation={0}>
              <CardContent style={{ paddingBottom: 12 }}>
                <Box className={classes.taskRow}>
                  <CircularProgress size={18} />
                  <Box className={classes.taskMain}>
                    <Typography className={classes.songName} noWrap>
                      {t.songName}{t.artist ? ` - ${t.artist}` : ''}
                    </Typography>
                    <Typography className={classes.sub}>
                      {statusLabel(t)}
                      {t.totalBytes > 0 ? ` · ${fmtSize(t.progressBytes)} / ${fmtSize(t.totalBytes)}` : ''}
                    </Typography>
                  </Box>
                  <Chip size="small" label={statusLabel(t)} variant="outlined" />
                </Box>
                <Box className={classes.progressRow}>
                  <LinearProgress variant="determinate" value={progressPct(t)} style={{ flex: 1 }} />
                  <Typography className={classes.sub}>{progressPct(t)}%</Typography>
                </Box>
              </CardContent>
            </Card>
          ))}

          {failedTasks.map((t) => (
            <Card key={t.id} className={`${classes.taskCard} ${classes.taskFailed}`} elevation={0}>
              <CardContent style={{ paddingBottom: 12 }}>
                <Box className={classes.taskRow}>
                  <ErrorIcon color="error" />
                  <Box className={classes.taskMain}>
                    <Typography className={classes.songName} noWrap>
                      {t.songName}{t.artist ? ` - ${t.artist}` : ''}
                    </Typography>
                    <Typography className={classes.sub} noWrap>{t.error || '下载失败'}</Typography>
                  </Box>
                  <Button size="small" startIcon={<RefreshIcon />} onClick={() => handleRetry(t)}>重试</Button>
                  <Button size="small" color="secondary" onClick={() => handleDeleteTask(t)}>删除</Button>
                </Box>
              </CardContent>
            </Card>
          ))}

          {finishedTasks.map((t) => (
            <Card key={t.id} className={`${classes.taskCard} ${classes.taskDone}`} elevation={0}>
              <CardContent style={{ paddingBottom: 12 }}>
                <Box className={classes.taskRow}>
                  <CheckCircleIcon style={{ color: '#2ed573' }} />
                  <Box className={classes.taskMain}>
                    <Typography className={classes.songName} noWrap>
                      {t.songName}{t.artist ? ` - ${t.artist}` : ''}
                    </Typography>
                    <Typography className={classes.sub} noWrap>
                      {t.actualFormat
                        ? `${t.actualFormat.toUpperCase()} ${t.actualBitrate || ''}k · 已入库，可播放`
                        : '已入库，可播放'}
                    </Typography>
                  </Box>
                  <Button size="small" color="secondary" onClick={() => handleDeleteTask(t)}>删除</Button>
                </Box>
              </CardContent>
            </Card>
          ))}
        </>
      )}
    </Box>
  )
}

export default OnlineSearchPage