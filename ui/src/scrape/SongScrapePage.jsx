import React, { useState, useEffect, useCallback } from 'react'
import {
  Typography, Box, makeStyles, Button, IconButton,
  Tooltip, Chip, TextField, Checkbox, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, LinearProgress,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import RefreshIcon from '@material-ui/icons/Refresh'
import BuildIcon from '@material-ui/icons/Build'
import SearchIcon from '@material-ui/icons/Search'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「歌曲刮削」页（M5.2，仅管理员）：对曲库中的任意歌曲刮削元数据
// （封面/歌词/标签，全内嵌重写）。支持多选批量刮削与单曲刮削，
// 批量请求之间随机间隔降低上游风控概率。

const PAGE_SIZE = 50

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1400, margin: '0 auto' },
  header: { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16, flexWrap: 'wrap' },
  filterRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginBottom: 16 },
  toolbar: { display: 'flex', gap: 12, alignItems: 'center', marginBottom: 12, flexWrap: 'wrap' },
  empty: { textAlign: 'center', padding: 48, color: theme.palette.text.secondary },
  pager: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 12, marginBottom: 24 },
  fieldLabel: { fontSize: 12, color: theme.palette.text.secondary },
  dialogCover: { width: 160, height: 160, borderRadius: 8, objectFit: 'cover', background: '#222' },
  dialogMain: { flex: 1, minWidth: 260 },
  editRow: { display: 'flex', gap: 12, flexWrap: 'wrap', marginTop: 12 },
  resultBox: { marginTop: 12 },
  progressText: { fontSize: 13, color: theme.palette.text.secondary },
}))

const fmtTime = (s) => {
  if (!s || s <= 0) return '-'
  const m = Math.floor(s / 60)
  const ss = String(s % 60).padStart(2, '0')
  return `${m}:${ss}`
}

const jitter = () => 800 + Math.floor(Math.random() * 1000)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const SongScrapePage = () => {
  const classes = useStyles()
  const [draft, setDraft] = useState({ title: '', artist: '' })
  const [query, setQuery] = useState({ title: '', artist: '', page: 1 })
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState(new Set())
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')

  // 批量进度
  const [batch, setBatch] = useState(null) // { done, total, current }

  // 单曲对话框
  const [dialog, setDialog] = useState(null) // { item, title, artist, album, result, busy }

  const fetchSongs = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams()
      if (query.title) params.set('title', query.title)
      if (query.artist) params.set('artist', query.artist)
      params.set('_start', String((query.page - 1) * PAGE_SIZE))
      params.set('_end', String(query.page * PAGE_SIZE))
      params.set('_sort', 'title')
      params.set('_order', 'ASC')
      const res = await httpClient(`${REST_URL}/song?${params.toString()}`)
      setItems(res.json || [])
      setTotal(parseInt(res.headers?.get?.('x-total-count') || '0', 10) || 0)
    } catch (e) {
      setError('加载曲库失败：' + (e.message || ''))
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [query])

  useEffect(() => {
    fetchSongs()
  }, [fetchSongs])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 5000)
    return () => clearTimeout(timer)
  }, [toast])

  const applyFilter = () => setQuery({ ...draft, page: 1 })

  const toggleSelect = (id) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = () => {
    setSelected((prev) => {
      const allSelected = items.length > 0 && items.every((it) => prev.has(it.id))
      return allSelected ? new Set() : new Set(items.map((it) => it.id))
    })
  }

  const scrapeOne = async (id, req) => {
    const res = await httpClient(`${REST_URL}/scrape/song/${id}/scrape`, {
      method: 'POST',
      body: JSON.stringify(req || {}),
    })
    return res.json
  }

  // 单曲刮削（对话框）
  const openDialog = (item) => {
    setDialog({
      item,
      title: item.title || '',
      artist: item.artist || '',
      album: item.album || '',
      result: null,
      busy: false,
    })
  }

  const handleDialogScrape = async () => {
    if (!dialog) return
    setDialog((d) => ({ ...d, busy: true }))
    setError('')
    try {
      const res = await scrapeOne(dialog.item.id, {
        title: dialog.title,
        artist: dialog.artist,
        album: dialog.album,
      })
      setDialog((d) => ({ ...d, result: res.result, busy: false }))
      setToast(`《${res.result?.info?.title || dialog.item.title}》刮削完成`)
      fetchSongs()
    } catch (e) {
      setDialog((d) => ({ ...d, busy: false }))
      setError('刮削失败：' + (e.message || ''))
    }
  }

  // 批量刮削（顺序执行 + 随机间隔）
  const handleBatchScrape = async () => {
    const ids = items.filter((it) => selected.has(it.id)).map((it) => it.id)
    if (ids.length === 0) return
    if (!window.confirm(`确定对选中的 ${ids.length} 首歌曲刮削元数据？\n将重写文件内嵌封面/歌词/标签（不动音频流）。`)) return
    setBatch({ done: 0, total: ids.length, current: '' })
    setError('')
    let ok = 0
    const failed = []
    for (let i = 0; i < ids.length; i += 1) {
      const item = items.find((it) => it.id === ids[i])
      setBatch({ done: i, total: ids.length, current: item ? `${item.title} - ${item.artist}` : ids[i] })
      try {
        await scrapeOne(ids[i], {})
        ok += 1
      } catch (e) {
        failed.push(item ? item.title : ids[i])
      }
      if (i < ids.length - 1) await sleep(jitter())
    }
    setBatch(null)
    const failNote = failed.length > 0 ? `，失败 ${failed.length} 首（${failed.slice(0, 3).join('、')}${failed.length > 3 ? '…' : ''}）` : ''
    setToast(`批量刮削完成：${ok}/${ids.length} 首成功${failNote}`)
    setSelected(new Set())
    fetchSongs()
  }

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const allSelected = items.length > 0 && items.every((it) => selected.has(it.id))

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <MusicNoteIcon style={{ fontSize: 28, color: '#1e90ff' }} />
        <Typography variant="h6" style={{ fontWeight: 700 }}>歌曲刮削</Typography>
        <Chip label={`共 ${total} 首`} size="small" variant="outlined" />
        <Typography style={{ fontSize: 12, color: '#888' }}>
          为曲库歌曲补齐/修正内嵌封面、歌词与标签
        </Typography>
      </Box>

      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}

      <Box className={classes.filterRow}>
        <TextField label="歌名" size="small" variant="outlined" value={draft.title}
          onChange={(e) => setDraft({ ...draft, title: e.target.value })}
          onKeyDown={(e) => e.key === 'Enter' && applyFilter()} />
        <TextField label="歌手" size="small" variant="outlined" value={draft.artist}
          onChange={(e) => setDraft({ ...draft, artist: e.target.value })}
          onKeyDown={(e) => e.key === 'Enter' && applyFilter()} />
        <Button variant="outlined" color="primary" startIcon={<SearchIcon />} onClick={applyFilter}>查询</Button>
        <Tooltip title="刷新">
          <IconButton onClick={fetchSongs}><RefreshIcon /></IconButton>
        </Tooltip>
      </Box>

      <Box className={classes.toolbar}>
        <Button variant="contained" color="primary" startIcon={<BuildIcon />}
          disabled={selected.size === 0 || !!batch}
          onClick={handleBatchScrape}>
          批量刮削选中（{selected.size}）
        </Button>
        <Typography className={classes.progressText}>
          批量刮削逐首执行并自动随机间隔；标题会自动清洗，多歌手按「A • B」格式入库
        </Typography>
      </Box>

      {batch && (
        <Box style={{ marginBottom: 12 }}>
          <LinearProgress variant="determinate" value={Math.round((batch.done / batch.total) * 100)} />
          <Typography className={classes.progressText}>
            刮削中 {batch.done + 1}/{batch.total}：{batch.current}
          </Typography>
        </Box>
      )}

      {loading ? (
        <LinearProgress />
      ) : items.length === 0 ? (
        <Box className={classes.empty}>
          <MusicNoteIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 12 }}>没有匹配的歌曲</Typography>
        </Box>
      ) : (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell padding="checkbox">
                  <Checkbox checked={allSelected} onChange={toggleSelectAll} color="primary" />
                </TableCell>
                <TableCell>歌名</TableCell>
                <TableCell>歌手</TableCell>
                <TableCell>专辑</TableCell>
                <TableCell>时长</TableCell>
                <TableCell align="right">操作</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id} hover selected={selected.has(item.id)}>
                  <TableCell padding="checkbox">
                    <Checkbox checked={selected.has(item.id)} onChange={() => toggleSelect(item.id)} color="primary" />
                  </TableCell>
                  <TableCell>{item.title}</TableCell>
                  <TableCell>{item.artist}</TableCell>
                  <TableCell>{item.album || '-'}</TableCell>
                  <TableCell>{fmtTime(item.duration)}</TableCell>
                  <TableCell align="right" style={{ whiteSpace: 'nowrap' }}>
                    <Button size="small" variant="outlined" color="primary" startIcon={<BuildIcon />}
                      disabled={!!batch} onClick={() => openDialog(item)}>
                      刮削
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {total > PAGE_SIZE && (
        <Box className={classes.pager}>
          <Typography className={classes.fieldLabel}>第 {query.page} / {pages} 页</Typography>
          <Button size="small" disabled={query.page <= 1}
            onClick={() => setQuery({ ...query, page: query.page - 1 })}>上一页</Button>
          <Button size="small" disabled={query.page >= pages}
            onClick={() => setQuery({ ...query, page: query.page + 1 })}>下一页</Button>
        </Box>
      )}

      {/* 单曲刮削对话框：可修正关键词后刮削 */}
      <Dialog open={!!dialog} onClose={() => setDialog(null)} maxWidth="md" fullWidth>
        <DialogTitle>歌曲刮削 · {dialog?.item?.title}</DialogTitle>
        <DialogContent>
          {dialog && (
            <>
              <Box className={classes.editRow} style={{ marginTop: 0 }}>
                <TextField label="标题（可修正）" size="small" variant="outlined" value={dialog.title}
                  onChange={(e) => setDialog({ ...dialog, title: e.target.value })} style={{ minWidth: 220 }} />
                <TextField label="歌手（可修正）" size="small" variant="outlined" value={dialog.artist}
                  onChange={(e) => setDialog({ ...dialog, artist: e.target.value })} style={{ minWidth: 220 }} />
                <TextField label="专辑（可修正）" size="small" variant="outlined" value={dialog.album}
                  onChange={(e) => setDialog({ ...dialog, album: e.target.value })} style={{ minWidth: 220 }} />
              </Box>
              <Typography style={{ fontSize: 12, color: '#888', marginTop: 8 }}>
                刮削会重新抓取封面/歌词并重写内嵌标签（不动音频流、不改文件名）。
                标题自动清洗（如「相思风雨中-《出位江湖》电视剧插曲」→「相思风雨中」），多歌手自动按「A • B」入库。
              </Typography>

              {dialog.result && (
                <Box className={classes.resultBox}>
                  <Alert severity={dialog.result.matched ? 'success' : 'warning'}>
                    {dialog.result.matched
                      ? `已匹配在线歌曲：${dialog.result.match?.name} - ${dialog.result.match?.artist}`
                      : '未找到匹配的在线歌曲，仅清洗并重写了标题/歌手标签'}
                  </Alert>
                  <Box style={{ display: 'flex', gap: 12, marginTop: 12, alignItems: 'flex-start' }}>
                    {dialog.result.info?.hasCover ? (
                      <Typography className={classes.fieldLabel}>🖼 封面已内嵌</Typography>
                    ) : (
                      <Typography className={classes.fieldLabel} style={{ color: '#ffa502' }}>🖼 无封面</Typography>
                    )}
                    <Typography className={classes.fieldLabel}>
                      {dialog.result.lyricsFetched ? '📝 歌词已抓取并内嵌' : dialog.result.info?.lyrics ? '📝 保留原歌词' : '📝 无歌词'}
                    </Typography>
                    <Typography className={classes.fieldLabel}>
                      入库标题：{dialog.result.info?.title || '-'}
                    </Typography>
                  </Box>
                </Box>
              )}
            </>
          )}
          {dialog?.busy && <LinearProgress style={{ marginTop: 12 }} />}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialog(null)}>关闭</Button>
          <Button color="primary" variant="contained" disabled={dialog?.busy}
            onClick={handleDialogScrape} startIcon={<BuildIcon />}>
            {dialog?.busy ? '刮削中…' : dialog?.result ? '再次刮削' : '开始刮削'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default SongScrapePage
