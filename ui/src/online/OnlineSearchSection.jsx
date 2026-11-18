import React, { useEffect, useState } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  Tooltip, Chip, TextField, Switch, FormControlLabel, MenuItem, LinearProgress,
  Radio, Collapse,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import GetAppIcon from '@material-ui/icons/GetApp'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import ExpandMoreIcon from '@material-ui/icons/ExpandMore'
import ExpandLessIcon from '@material-ui/icons/ExpandLess'
import { Alert } from '@material-ui/lab'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「搜歌下载」区：单输入框批量搜索（每行一首，回车分割）+ 分组结果 + 一键批量下载。
// 批量搜索/下载请求之间加随机间隔，降低上游风控概率。

const useStyles = makeStyles((theme) => ({
  formCard: { marginBottom: 16 },
  formRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginTop: 12 },
  hint: { fontSize: 12, color: theme.palette.text.secondary, marginTop: 8 },
  groupCard: { marginBottom: 16 },
  groupHeader: { display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 },
  groupTitle: { fontWeight: 700, fontSize: 15 },
  resultRow: { display: 'flex', alignItems: 'center', gap: 12, padding: '8px 0', borderTop: `1px solid ${theme.palette.divider}` },
  cover: { width: 52, height: 52, borderRadius: 4, objectFit: 'cover', background: '#222', flexShrink: 0 },
  resultMain: { flex: 1, minWidth: 0 },
  songName: { fontWeight: 600, fontSize: 14 },
  sub: { fontSize: 12, color: theme.palette.text.secondary },
  chip: { marginRight: 6, marginBottom: 4 },
  inLibrary: { opacity: 0.55 },
  empty: { textAlign: 'center', padding: 40, color: theme.palette.text.secondary },
  batchNote: { fontSize: 12, color: '#888' },
  expandBtn: { marginTop: 6 },
  pickRow: { display: 'flex', alignItems: 'center', gap: 4 },
}))

const QUALITY_OPTIONS = [
  { value: '', label: '默认（无损优先）' },
  { value: 'flac/2000', label: 'FLAC 2000k' },
  { value: 'mp3/320', label: 'MP3 320k' },
  // 注：不再提供 128k 选项——上游把 128k 当“试听档”返回 12 秒占位片段而非真曲
]

const fmtDuration = (s) => {
  if (!s || s <= 0) return ''
  const m = Math.floor(s / 60)
  const ss = String(s % 60).padStart(2, '0')
  return `${m}:${ss}`
}

const qualityLabel = (q) => {
  const fmt = (q.format || '').toUpperCase()
  const parts = [fmt, q.bitrate ? `${q.bitrate}k` : ''].filter(Boolean)
  const head = parts.join(' ')
  return q.size ? `${head} ${q.size}` : head
}

// 随机间隔：800–1800ms，批量请求之间使用，避免固定节奏触发风控
const jitter = () => 800 + Math.floor(Math.random() * 1000)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// parseQueries 把多行输入解析为 [{name, artist}]。
// 每行格式：`歌名 - 歌手` / `歌名-歌手` / `歌名 歌手` / 只有歌名。
const parseQueries = (text) => {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const dash = line.match(/^(.*?)[-—–－]\s*(.+)$/)
      if (dash && dash[1].trim()) {
        return { name: dash[1].trim(), artist: dash[2].trim() }
      }
      const sp = line.match(/^(\S+)\s+(.+)$/)
      if (sp) {
        return { name: sp[1].trim(), artist: sp[2].trim() }
      }
      return { name: line, artist: '' }
    })
}

const songKey = (song) => `${song.source}-${song.id}`

// react-admin 的 fetchJson 把响应正文放在 error.body，message 只有状态文本
const errorText = (e) => {
  const raw = (e && (e.body || e.message)) || ''
  if (typeof raw === 'string') return raw.trim() || '请求失败'
  if (raw && typeof raw === 'object') return raw.error || raw.message || JSON.stringify(raw)
  return '请求失败'
}

// 默认选中：优先未收录且未在下载中的首个结果，否则（模糊模式）选首个可下载的，再否则不选
const defaultPick = (songs, fuzzy) => {
  const idle = songs.filter((x) => !x.downloading)
  const s = idle.find((x) => !x.inLibrary) || (fuzzy ? idle[0] : null)
  return s ? songKey(s) : null
}

const MAX_VISIBLE = 3 // 每组默认只显示前 3 条，避免批量搜索时列表过长

const OnlineSearchSection = ({ onTasksChanged }) => {
  const classes = useStyles()
  const [text, setText] = useState('')
  const [fuzzy, setFuzzy] = useState(true)
  const [quality, setQuality] = useState('')
  const [searching, setSearching] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const [groups, setGroups] = useState([])
  // groups: [{ query, songs, total, state, error, pick, expanded }]
  //   pick = 选中版本的 `${source}-${id}`（批量下载时使用）；expanded = 是否展开全部结果
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')
  // 试听：全局同时只播一首；audioRef 单例，playingKey 记录当前试听的歌曲
  const [playingKey, setPlayingKey] = useState('')
  const audioRef = React.useRef(null)
  // 下载权限（服务端 settings 接口）：非管理员在「仅管理员可下载」模式下禁用下载按钮
  const [access, setAccess] = useState(null)

  useEffect(() => {
    let cancelled = false
    const loadAccess = async () => {
      try {
        const res = await httpClient(`${REST_URL}/online/settings`)
        if (!cancelled) setAccess(res.json || null)
      } catch (e) {
        // 读取失败不阻塞搜索；下载时后端仍会校验
        if (!cancelled) setAccess(null)
      }
    }
    loadAccess()
    return () => { cancelled = true }
  }, [])

  const canDownload = !access || access.canDownload !== false

  // ---- 试听 ----
  const stopPreview = () => {
    const a = audioRef.current
    if (a) {
      a.pause()
      a.src = ''
    }
    setPlayingKey('')
  }

  useEffect(() => () => stopPreview(), [])

  // 解析试听直链并播放；再点同一首 = 停止。128k 是上游占位片段，统一用320k（真曲）。
  const handlePreview = async (song) => {
    const key = songKey(song)
    if (playingKey === key) {
      stopPreview()
      return
    }
    stopPreview()
    try {
      const res = await httpClient(`${REST_URL}/online/preview`, {
        method: 'POST',
        body: JSON.stringify({
          source: song.source,
          platform: song.platform,
          songId: song.id,
          time: song.time,
          sign: song.sign,
          duration: song.duration, // 期望时长：服务端据此拦截上游广告占位音频
        }),
      })
      const url = res.json?.url
      if (!url) throw new Error('未获取到试听地址')
      const a = audioRef.current || new Audio()
      a.src = url
      a.onended = () => setPlayingKey('')
      a.onerror = () => {
        setPlayingKey('')
        setError('试听播放失败（网络或链接过期）')
      }
      audioRef.current = a
      await a.play()
      setPlayingKey(key)
    } catch (e) {
      stopPreview()
      setError('试听失败：' + errorText(e))
    }
  }

  const handleSearch = async () => {
    const queries = parseQueries(text)
    if (queries.length === 0) return
    setSearching(true)
    setError('')
    setGroups(queries.map((q) => ({ query: q, songs: [], total: 0, state: 'pending' })))
    try {
      for (let i = 0; i < queries.length; i += 1) {
        const q = queries[i]
        try {
          const keyword = [q.name, q.artist].filter(Boolean).join(' ')
          const res = await httpClient(
            `${REST_URL}/online/search?keyword=${encodeURIComponent(keyword)}&page=1&size=20`,
          )
          const songs = res.json?.songs || []
          setGroups((gs) => gs.map((g, idx) => (idx === i
            ? {
              ...g,
              songs,
              total: res.json?.total || 0,
              state: 'done',
              pick: defaultPick(songs, fuzzy),
              expanded: false,
            }
            : g)))
        } catch (e) {
          setGroups((gs) => gs.map((g, idx) => (idx === i
            ? { ...g, state: 'error', error: e.message || '搜索失败' }
            : g)))
        }
        // 多首之间随机间隔，降低风控概率
        if (i < queries.length - 1) await sleep(jitter())
      }
    } finally {
      setSearching(false)
    }
  }

  const downloadOne = async (song) => {
    await httpClient(`${REST_URL}/online/download`, {
      method: 'POST',
      body: JSON.stringify({
        source: song.source,
        platform: song.platform,
        songId: song.id,
        name: song.name,
        artist: song.artist,
        album: song.albumName,
        albumId: song.albumId,
        coverUrl: song.coverUrl,
        duration: song.duration, // 期望时长：服务端据此拦截上游广告占位音频
        quality,
        fuzzy,
        time: song.time,
        sign: song.sign,
      }),
    })
  }

  // 局部更新某一行的状态（下载中标记、失败回退等）
  const patchSong = (song, patch) => {
    setGroups((gs) => gs.map((g) => ({
      ...g,
      songs: (g.songs || []).map((s) => (songKey(s) === songKey(song) ? { ...s, ...patch } : s)),
    })))
  }

  // 已在队列中的版本：本地立即标记，避免连点重复提交（服务端也有去重拦截）
  const markPending = (song) => patchSong(song, { downloading: true })

  const handleDownload = async (song) => {
    if (song.downloading) return
    setError('')
    markPending(song)
    try {
      await downloadOne(song)
      setToast(`《${song.name}》已加入下载队列`)
      if (onTasksChanged) setTimeout(onTasksChanged, 600)
    } catch (e) {
      const msg = errorText(e)
      if (e && e.status === 409) {
        // 已在曲库/已在下载：不再允许点击，提示用户到下载管理处理
        patchSong(song, { inLibrary: true, downloading: false })
        setError(msg)
      } else {
        setError('下载失败：' + msg)
        patchSong(song, { downloading: false })
      }
    }
  }

  // 再次点击已选版本即取消选择（批量下载会跳过该首）
  const setPick = (gi, key) => {
    setGroups((gs) => gs.map((g, idx) => (idx === gi ? { ...g, pick: g.pick === key ? null : key } : g)))
  }

  const toggleExpand = (gi) => {
    setGroups((gs) => gs.map((g, idx) => (idx === gi ? { ...g, expanded: !g.expanded } : g)))
  }

  // 批量下载：每组下载「选中版本」（单选按钮指定，默认自动选未收录的首个）
  const handleDownloadAll = async () => {
    const picks = []
    let skippedLibrary = 0
    let skippedPending = 0
    let noPick = 0
    groups.forEach((g) => {
      const pick = (g.songs || []).find((s) => songKey(s) === g.pick)
      if (!pick) {
        noPick += 1
        return
      }
      // 已在下载队列里的不再重复提交
      if (pick.downloading) {
        skippedPending += 1
        return
      }
      // 已在曲库的版本（非模糊模式）服务端也会拒绝，这里提前跳过
      if (pick.inLibrary && !fuzzy) {
        skippedLibrary += 1
        return
      }
      picks.push({ pick })
    })
    if (picks.length === 0) {
      if (skippedPending > 0) setError(`${skippedPending} 首已在下载队列中，无需重复下载`)
      else if (skippedLibrary > 0) setError('所选歌曲均已在曲库（可开启模糊匹配下载其他版本，或点选其他版本）')
      else if (noPick > 0) setError('所选歌曲均已在曲库，批量下载已跳过')
      else setError('请先搜索歌曲')
      return
    }
    setDownloading(true)
    setError('')
    let ok = 0
    const failures = []
    try {
      for (let i = 0; i < picks.length; i += 1) {
        const song = picks[i].pick
        markPending(song)
        try {
          await downloadOne(song)
          ok += 1
        } catch (e) {
          // 单首失败不中断批量，继续后面的
          const msg = errorText(e)
          failures.push(`《${song.name}》：${msg}`)
          patchSong(song, e && e.status === 409 ? { inLibrary: true, downloading: false } : { downloading: false })
        }
        if (i < picks.length - 1) await sleep(jitter())
      }
      const notes = []
      if (skippedLibrary > 0) notes.push(`${skippedLibrary} 首已在曲库跳过`)
      if (skippedPending > 0) notes.push(`${skippedPending} 首已在下载队列跳过`)
      if (noPick > 0) notes.push(`${noPick} 组未选版本跳过`)
      const skippedNote = notes.length ? `，${notes.join('，')}` : ''
      setToast(`批量下载完成：${ok}/${picks.length} 首已加入队列${skippedNote}`)
      if (failures.length > 0) {
        const head = failures.slice(0, 3).join('；')
        const more = failures.length > 3 ? `；等共 ${failures.length} 首失败` : ''
        setError(`部分歌曲未加入队列：${head}${more}`)
      }
      if (onTasksChanged) setTimeout(onTasksChanged, 600)
    } finally {
      setDownloading(false)
    }
  }

  // 下载按钮禁用原因（作为 Tooltip 文案）
  const downloadBlockReason = (song) => {
    if (!canDownload) return '当前账号没有下载权限'
    if (song.downloading) return '已加入下载队列，可到「下载任务」查看进度'
    if (song.inLibrary && !fuzzy) return '曲库已有这首歌，需开启模糊匹配才能下载其他版本'
    return '加入下载队列'
  }

  return (
    <>
      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}
      {!canDownload && (
        <Alert severity="info" style={{ marginBottom: 12 }}>
          当前下载权限为「仅管理员可下载」，普通用户只能搜索查看。
          如需开放，请由管理员在「权限设置」中调整。
        </Alert>
      )}

      <Card className={classes.formCard} elevation={1}>
        <CardContent>
          <TextField
            label="搜歌（每行一首，回车分割）"
            placeholder={'相思风雨中 - 张学友\n海阔天空 BEYOND\n吻别\n……格式：歌名 - 歌手，或歌名 空格 歌手，也可只写歌名'}
            multiline
            rows={4}
            fullWidth
            variant="outlined"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <Box className={classes.formRow}>
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
              onClick={handleSearch} disabled={searching || downloading}>
              {searching ? '搜索中…' : '搜索'}
            </Button>
            <Button variant="contained" color="secondary" startIcon={<GetAppIcon />}
              onClick={handleDownloadAll} disabled={searching || downloading || groups.length === 0 || !canDownload}>
              {downloading ? '批量下载中…' : '批量下载'}
            </Button>
            <Typography className={classes.batchNote}>
              批量搜索/下载自动随机间隔，降低被风控概率
            </Typography>
          </Box>
          <Typography className={classes.hint}>
            提示：标题带「电视剧插曲」「Live」等后缀会自动清洗为干净标题；多歌手会按曲库格式（A • B）入库。
            每组点选要下载的版本，再次点击可取消，批量下载会自动跳过未选中的歌曲。
          </Typography>
        </CardContent>
      </Card>

      {searching && groups.length > 0 && <LinearProgress style={{ marginBottom: 12 }} />}

      {groups.length === 0 ? (
        <Box className={classes.empty}>
          <SearchIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 12 }}>输入歌曲开始搜索，支持一次搜多首</Typography>
        </Box>
      ) : (
        groups.map((g, gi) => (
          <Card className={classes.groupCard} elevation={0} key={`g${gi}`}>
            <CardContent style={{ paddingBottom: 8 }}>
              <Box className={classes.groupHeader}>
                <Typography className={classes.groupTitle}>
                  《{g.query.name}》{g.query.artist ? ` - ${g.query.artist}` : ''}
                </Typography>
                <Chip size="small" variant="outlined"
                  label={g.state === 'pending' ? '排队中' : g.state === 'error' ? '搜索失败' : `共 ${g.total} 条`} />
              </Box>
              {g.state === 'error' && <Alert severity="warning">{g.error}</Alert>}
              {g.state === 'done' && g.songs.length === 0 && (
                <Typography className={classes.sub}>没有找到匹配的歌曲，试试开启「模糊匹配」</Typography>
              )}
              {(g.songs || []).slice(0, g.expanded ? undefined : MAX_VISIBLE).map((song) => (
                <Box className={classes.resultRow} key={songKey(song)}>
                  <Box className={classes.pickRow}>
                    <Radio
                      checked={g.pick === songKey(song)}
                      onClick={() => setPick(gi, songKey(song))}
                      color="primary"
                      size="small"
                    />
                  </Box>
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
                      <Chip size="small" label={song.platform || song.source} className={classes.chip} />
                      {song.duration > 0 && (
                        <Chip size="small" variant="outlined" label={fmtDuration(song.duration)} className={classes.chip} />
                      )}
                      {(song.qualities || []).slice(0, 3).map((q) => (
                        <Chip key={`${q.format}-${q.bitrate}-${q.level || ''}`} size="small" variant="outlined"
                          label={qualityLabel(q)} className={classes.chip} />
                      ))}
                      {song.inLibrary && (
                        <Chip size="small" color="primary" label="已在曲库" className={classes.chip} />
                      )}
                      {song.downloading && (
                        <Chip size="small" label="已加入下载队列" className={classes.chip}
                          style={{ background: '#1e90ff', color: '#fff' }} />
                      )}
                      {g.pick === songKey(song) && !song.downloading && (
                        <Chip size="small" color="secondary" label="将下载" className={classes.chip} />
                      )}
                    </Box>
                  </Box>
                  <Box style={{ display: 'flex', gap: 8, flexShrink: 0, alignItems: 'center' }}>
                    <Tooltip title={playingKey === songKey(song) ? '停止试听' : '试听（在线播放）'}>
                      <Button
                        variant="outlined"
                        size="small"
                        color={playingKey === songKey(song) ? 'secondary' : 'default'}
                        startIcon={<PlayArrowIcon />}
                        onClick={() => handlePreview(song)}
                      >
                        {playingKey === songKey(song) ? '停止' : '试听'}
                      </Button>
                    </Tooltip>
                    <Tooltip title={downloadBlockReason(song)}>
                      <span>
                        <Button variant="contained" size="small" color="primary" startIcon={<GetAppIcon />}
                          disabled={downloading || !canDownload || song.downloading || (song.inLibrary && !fuzzy)}
                          onClick={() => handleDownload(song)}>
                          {song.downloading ? '已下载' : '下载'}
                        </Button>
                      </span>
                    </Tooltip>
                  </Box>
                </Box>
              ))}
              {(g.songs || []).length > MAX_VISIBLE && (
                <Button
                  size="small"
                  className={classes.expandBtn}
                  startIcon={g.expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                  onClick={() => toggleExpand(gi)}
                >
                  {g.expanded ? '收起' : `展开全部 ${g.songs.length} 条`}
                </Button>
              )}
              {g.state === 'done' && g.songs.length > 0 && (
                <Typography className={classes.batchNote} style={{ marginTop: 4 }}>
                  {g.pick
                    ? '点选左侧单选按钮可切换批量下载的版本'
                    : '未选中任何版本（均已在曲库），批量下载时将跳过'}
                </Typography>
              )}
            </CardContent>
          </Card>
        ))
      )}
    </>
  )
}

export default OnlineSearchSection
