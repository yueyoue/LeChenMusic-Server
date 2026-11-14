import React, { useState, useEffect, useCallback } from 'react'
import {
  Typography, Box, makeStyles, Button, IconButton,
  Tooltip, Chip, TextField, MenuItem, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, LinearProgress,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import StorageIcon from '@material-ui/icons/Storage'
import RefreshIcon from '@material-ui/icons/Refresh'
import DeleteIcon from '@material-ui/icons/Delete'
import BuildIcon from '@material-ui/icons/Build'
import VisibilityIcon from '@material-ui/icons/Visibility'
import HistoryIcon from '@material-ui/icons/History'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「下载管理」页（M5，仅管理员）：查看所有用户下载的歌曲，验证刮削是否准确，
// 直接删除下载的歌曲 / 对刮削不准确的歌曲重新刮削；内置操作日志。

const PAGE_SIZE = 50

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1400, margin: '0 auto' },
  header: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16, flexWrap: 'wrap', gap: 8 },
  headerLeft: { display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' },
  filterRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginBottom: 16 },
  tableWrap: { marginBottom: 12 },
  pathCell: { maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 12, fontFamily: 'monospace' },
  okChip: { background: '#2ed573', color: '#fff' },
  warnChip: { background: '#ffa502', color: '#fff' },
  empty: { textAlign: 'center', padding: 48, color: theme.palette.text.secondary },
  pager: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 12, marginBottom: 24 },
  verifyBox: { display: 'flex', gap: 24, flexWrap: 'wrap' },
  verifyCover: { width: 180, height: 180, borderRadius: 8, objectFit: 'cover', background: '#222' },
  verifyMain: { flex: 1, minWidth: 260 },
  fieldRow: { marginBottom: 8 },
  fieldLabel: { fontSize: 12, color: theme.palette.text.secondary },
  fieldValue: { fontSize: 14, fontWeight: 500, wordBreak: 'break-all' },
  lyricsBox: { maxHeight: 240, overflow: 'auto', whiteSpace: 'pre-wrap', fontSize: 13, background: 'rgba(0,0,0,0.2)', padding: 12, borderRadius: 6, fontFamily: 'monospace' },
  editRow: { display: 'flex', gap: 12, flexWrap: 'wrap', marginTop: 12 },
}))

const SCRAPE_FILTERS = [
  { value: '', label: '全部刮削状态' },
  { value: 'ok', label: '刮削完整' },
  { value: 'missing-cover', label: '缺封面' },
  { value: 'missing-lyrics', label: '缺歌词' },
  { value: 'missing-tags', label: '缺标签' },
]

const LOG_TYPES = [
  { value: '', label: '全部操作类型' },
  { value: 'download_queued', label: '加入下载' },
  { value: 'download_done', label: '下载完成' },
  { value: 'download_failed', label: '下载失败' },
  { value: 'delete', label: '删除歌曲' },
  { value: 'rescrape', label: '重新刮削' },
]

const fmtTime = (v) => (v && !v.startsWith('0001-') ? new Date(v).toLocaleString() : '-')

const opLabel = (action) => {
  switch (action) {
    case 'download_queued': return '加入下载'
    case 'download_done': return '下载完成'
    case 'download_failed': return '下载失败'
    case 'delete': return '删除歌曲'
    case 'rescrape': return '重新刮削'
    default: return action
  }
}

const Field = ({ label, value }) => {
  const classes = useStyles()
  return (
    <Box className={classes.fieldRow}>
      <Typography className={classes.fieldLabel}>{label}</Typography>
      <Typography className={classes.fieldValue}>{value}</Typography>
    </Box>
  )
}

const DownloadManagePage = () => {
  const classes = useStyles()
  const [tab, setTab] = useState('downloads') // downloads | logs

  // ---- 下载记录 ----
  const [dlDraft, setDlDraft] = useState({ q: '', user: '', scrape: '' })
  const [dlQuery, setDlQuery] = useState({ q: '', user: '', scrape: '', page: 1 })
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)

  // ---- 操作日志 ----
  const [logDraft, setLogDraft] = useState({ type: '', user: '', from: '', to: '' })
  const [logQuery, setLogQuery] = useState({ type: '', user: '', from: '', to: '', page: 1 })
  const [logs, setLogs] = useState([])
  const [logTotal, setLogTotal] = useState(0)
  const [logPages, setLogPages] = useState(0)
  const [logLoading, setLogLoading] = useState(false)

  // ---- 通用 ----
  const [verify, setVerify] = useState(null) // verify dialog state
  const [editTitle, setEditTitle] = useState('')
  const [editArtist, setEditArtist] = useState('')
  const [editAlbum, setEditAlbum] = useState('')
  const [rescraping, setRescraping] = useState(false)
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')

  const fetchDownloads = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams()
      if (dlQuery.q) params.set('q', dlQuery.q)
      if (dlQuery.user) params.set('user', dlQuery.user)
      if (dlQuery.scrape) params.set('scrape', dlQuery.scrape)
      params.set('offset', String((dlQuery.page - 1) * PAGE_SIZE))
      params.set('limit', String(PAGE_SIZE))
      const res = await httpClient(`${REST_URL}/online/downloads?${params.toString()}`)
      setItems(res.json?.data || [])
      setTotal(res.json?.total || 0)
    } catch (e) {
      setError('加载下载记录失败：' + (e.message || ''))
      setItems([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [dlQuery])

  const fetchLogs = useCallback(async () => {
    setLogLoading(true)
    try {
      const params = new URLSearchParams()
      if (logQuery.type) params.set('type', logQuery.type)
      if (logQuery.user) params.set('user', logQuery.user)
      if (logQuery.from) params.set('from', logQuery.from)
      if (logQuery.to) params.set('to', logQuery.to)
      params.set('page', String(logQuery.page))
      params.set('limit', String(PAGE_SIZE))
      const res = await httpClient(`${REST_URL}/online/logs?${params.toString()}`)
      setLogs(res.json?.data || [])
      setLogTotal(res.json?.total || 0)
      setLogPages(res.json?.pages || 0)
    } catch (e) {
      setError('加载操作日志失败：' + (e.message || ''))
      setLogs([])
      setLogTotal(0)
      setLogPages(0)
    } finally {
      setLogLoading(false)
    }
  }, [logQuery])

  useEffect(() => {
    if (tab === 'downloads') fetchDownloads()
    else fetchLogs()
  }, [tab, fetchDownloads, fetchLogs])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 4000)
    return () => clearTimeout(timer)
  }, [toast])

  const applyDlFilter = () => setDlQuery({ ...dlDraft, page: 1 })
  const applyLogFilter = () => setLogQuery({ ...logDraft, page: 1 })

  const openVerify = async (item) => {
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/downloads/${item.id}/verify`)
      setVerify(res.json)
      setEditTitle(res.json?.task?.songName || '')
      setEditArtist(res.json?.task?.artist || '')
      setEditAlbum(res.json?.task?.albumName || '')
    } catch (e) {
      setError('读取文件元数据失败：' + (e.message || ''))
    }
  }

  const handleDelete = async (item) => {
    if (!window.confirm(`确定删除下载的歌曲《${item.songName} - ${item.artist}》？\n将同时删除音频文件、曲库记录与任务记录。`)) return
    setError('')
    try {
      await httpClient(`${REST_URL}/online/downloads/${item.id}`, { method: 'DELETE' })
      setToast(`《${item.songName}》已删除`)
      fetchDownloads()
    } catch (e) {
      setError('删除失败：' + (e.message || ''))
    }
  }

  const handleRescrape = async () => {
    if (!verify) return
    setRescraping(true)
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/downloads/${verify.task.id}/rescrape`, {
        method: 'POST',
        body: JSON.stringify({
          title: editTitle,
          artist: editArtist,
          album: editAlbum,
        }),
      })
      const result = res.json
      setVerify((v) => ({
        ...v,
        metadata: result?.info || v.metadata,
        task: { ...v.task, songName: editTitle, artist: editArtist, albumName: editAlbum },
      }))
      setToast('重新刮削完成')
      fetchDownloads()
    } catch (e) {
      setError('重新刮削失败：' + (e.message || ''))
    } finally {
      setRescraping(false)
    }
  }

  const handleClearLogs = async () => {
    if (!window.confirm('确定清空操作日志？（不影响下载记录）')) return
    setError('')
    try {
      await httpClient(`${REST_URL}/online/logs`, { method: 'DELETE' })
      setToast('操作日志已清空')
      if (logQuery.page !== 1) setLogQuery((q) => ({ ...q, page: 1 }))
      else fetchLogs()
    } catch (e) {
      setError('清空日志失败：' + (e.message || ''))
    }
  }

  const scrapeBadge = (item) => {
    if (!item.fileExists) return <Chip label="文件缺失" size="small" className={classes.warnChip} />
    const s = item.scrape || {}
    return (
      <>
        <Chip label={s.hasCover ? '封面✓' : '封面✗'} size="small" style={{ marginRight: 4, marginBottom: 4, background: s.hasCover ? '#2ed573' : '#ffa502', color: '#fff' }} />
        <Chip label={s.lyrics ? '歌词✓' : '歌词✗'} size="small" style={{ marginRight: 4, marginBottom: 4, background: s.lyrics ? '#2ed573' : '#ffa502', color: '#fff' }} />
        <Chip label={s.title || s.artist ? '标签✓' : '标签✗'} size="small" style={{ marginBottom: 4, background: (s.title || s.artist) ? '#2ed573' : '#ffa502', color: '#fff' }} />
      </>
    )
  }

  const dlPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <Box className={classes.headerLeft}>
          <StorageIcon style={{ fontSize: 28, color: '#1e90ff' }} />
          <Typography variant="h6" style={{ fontWeight: 700 }}>下载管理</Typography>
          <Chip label={`共 ${tab === 'downloads' ? total : logTotal} 条`} size="small" variant="outlined" />
          <Button size="small" variant={tab === 'downloads' ? 'contained' : 'text'} color="primary"
            onClick={() => setTab('downloads')}>下载记录</Button>
          <Button size="small" variant={tab === 'logs' ? 'contained' : 'text'} color="primary"
            startIcon={<HistoryIcon />} onClick={() => setTab('logs')}>操作日志</Button>
        </Box>
        <Tooltip title="刷新">
          <IconButton onClick={tab === 'downloads' ? fetchDownloads : fetchLogs}><RefreshIcon /></IconButton>
        </Tooltip>
      </Box>

      {toast && <Alert severity="success" style={{ marginBottom: 12 }}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }}>{error}</Alert>}

      {tab === 'downloads' && (
        <>
          <Box className={classes.filterRow}>
            <TextField label="搜索歌名/歌手/专辑" size="small" variant="outlined" value={dlDraft.q}
              onChange={(e) => setDlDraft({ ...dlDraft, q: e.target.value })}
              onKeyDown={(e) => e.key === 'Enter' && applyDlFilter()} />
            <TextField label="下载用户" size="small" variant="outlined" value={dlDraft.user}
              onChange={(e) => setDlDraft({ ...dlDraft, user: e.target.value })}
              onKeyDown={(e) => e.key === 'Enter' && applyDlFilter()} />
            <TextField select label="刮削状态" size="small" variant="outlined" value={dlDraft.scrape}
              onChange={(e) => setDlDraft({ ...dlDraft, scrape: e.target.value })} style={{ minWidth: 160 }}>
              {SCRAPE_FILTERS.map((o) => <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>)}
            </TextField>
            <Button variant="outlined" color="primary" onClick={applyDlFilter}>查询</Button>
          </Box>

          {loading ? (
            <LinearProgress />
          ) : items.length === 0 ? (
            <Box className={classes.empty}>
              <StorageIcon style={{ fontSize: 48, opacity: 0.2 }} />
              <Typography style={{ marginTop: 12 }}>暂无下载记录</Typography>
            </Box>
          ) : (
            <TableContainer component={Paper} className={classes.tableWrap}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>歌名</TableCell>
                    <TableCell>歌手</TableCell>
                    <TableCell>专辑</TableCell>
                    <TableCell>下载用户</TableCell>
                    <TableCell>实际音质</TableCell>
                    <TableCell>文件路径</TableCell>
                    <TableCell>刮削状态</TableCell>
                    <TableCell>下载时间</TableCell>
                    <TableCell>完成时间</TableCell>
                    <TableCell align="right">操作</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item.id} hover>
                      <TableCell>{item.songName}</TableCell>
                      <TableCell>{item.artist}</TableCell>
                      <TableCell>{item.albumName || '-'}</TableCell>
                      <TableCell>{item.userName}</TableCell>
                      <TableCell>{item.actualFormat ? `${item.actualFormat.toUpperCase()} ${item.actualBitrate || ''}k` : '-'}</TableCell>
                      <TableCell>
                        <Tooltip title={item.filePath || '（无路径）'}>
                          <div className={classes.pathCell}>{item.filePath || '-'}</div>
                        </Tooltip>
                      </TableCell>
                      <TableCell>{scrapeBadge(item)}</TableCell>
                      <TableCell>{fmtTime(item.createdAt)}</TableCell>
                      <TableCell>{fmtTime(item.finishedAt)}</TableCell>
                      <TableCell align="right" style={{ whiteSpace: 'nowrap' }}>
                        <Tooltip title="验证刮削结果">
                          <IconButton size="small" onClick={() => openVerify(item)}><VisibilityIcon fontSize="small" /></IconButton>
                        </Tooltip>
                        <Tooltip title="重新刮削">
                          <IconButton size="small" color="primary" onClick={() => openVerify(item)}><BuildIcon fontSize="small" /></IconButton>
                        </Tooltip>
                        <Tooltip title="删除下载的歌曲">
                          <IconButton size="small" color="secondary" onClick={() => handleDelete(item)}><DeleteIcon fontSize="small" /></IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}

          {total > PAGE_SIZE && (
            <Box className={classes.pager}>
              <Typography className={classes.fieldLabel}>
                第 {dlQuery.page} / {dlPages} 页
              </Typography>
              <Button size="small" disabled={dlQuery.page <= 1}
                onClick={() => setDlQuery({ ...dlQuery, page: dlQuery.page - 1 })}>上一页</Button>
              <Button size="small" disabled={dlQuery.page >= dlPages}
                onClick={() => setDlQuery({ ...dlQuery, page: dlQuery.page + 1 })}>下一页</Button>
            </Box>
          )}
        </>
      )}

      {tab === 'logs' && (
        <>
          <Box className={classes.filterRow}>
            <TextField select label="操作类型" size="small" variant="outlined" value={logDraft.type}
              onChange={(e) => setLogDraft({ ...logDraft, type: e.target.value })} style={{ minWidth: 170 }}>
              {LOG_TYPES.map((o) => <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>)}
            </TextField>
            <TextField label="用户" size="small" variant="outlined" value={logDraft.user}
              onChange={(e) => setLogDraft({ ...logDraft, user: e.target.value })}
              onKeyDown={(e) => e.key === 'Enter' && applyLogFilter()} />
            <TextField label="开始日期" type="date" size="small" variant="outlined"
              InputLabelProps={{ shrink: true }} value={logDraft.from}
              onChange={(e) => setLogDraft({ ...logDraft, from: e.target.value })} />
            <TextField label="结束日期" type="date" size="small" variant="outlined"
              InputLabelProps={{ shrink: true }} value={logDraft.to}
              onChange={(e) => setLogDraft({ ...logDraft, to: e.target.value })} />
            <Button variant="outlined" color="primary" onClick={applyLogFilter}>查询</Button>
            <Button size="small" color="secondary" onClick={handleClearLogs}>清空日志</Button>
          </Box>

          {logLoading ? (
            <LinearProgress />
          ) : logs.length === 0 ? (
            <Box className={classes.empty}><Typography>暂无操作日志</Typography></Box>
          ) : (
            <TableContainer component={Paper}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>时间</TableCell>
                    <TableCell>操作</TableCell>
                    <TableCell>操作人</TableCell>
                    <TableCell>歌曲</TableCell>
                    <TableCell>结果</TableCell>
                    <TableCell>详情</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {logs.map((l) => (
                    <TableRow key={l.id} hover>
                      <TableCell>{fmtTime(l.createdAt)}</TableCell>
                      <TableCell>{opLabel(l.action)}</TableCell>
                      <TableCell>{l.userName}</TableCell>
                      <TableCell>{l.songName} {l.artist ? `- ${l.artist}` : ''}</TableCell>
                      <TableCell>
                        <Chip label={l.success ? '成功' : '失败'} size="small"
                          style={{ background: l.success ? '#2ed573' : '#ff4757', color: '#fff' }} />
                      </TableCell>
                      <TableCell style={{ maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        <Tooltip title={l.detail || ''}>
                          <div className={classes.pathCell} style={{ fontFamily: 'inherit' }}>{l.detail || '-'}</div>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}

          {logPages > 1 && (
            <Box className={classes.pager}>
              <Typography className={classes.fieldLabel}>
                第 {logQuery.page} / {logPages} 页 · 共 {logTotal} 条
              </Typography>
              <Button size="small" disabled={logQuery.page <= 1}
                onClick={() => setLogQuery({ ...logQuery, page: logQuery.page - 1 })}>上一页</Button>
              <Button size="small" disabled={logQuery.page >= logPages}
                onClick={() => setLogQuery({ ...logQuery, page: logQuery.page + 1 })}>下一页</Button>
            </Box>
          )}
        </>
      )}

      {/* 验证/重新刮削对话框 */}
      <Dialog open={!!verify} onClose={() => setVerify(null)} maxWidth="md" fullWidth>
        <DialogTitle>刮削验证 · {verify?.task?.songName}</DialogTitle>
        <DialogContent>
          {verify && (
            <>
              {!verify.exists ? (
                <Alert severity="warning">文件不存在：{verify.task?.filePath}</Alert>
              ) : (
                <Box className={classes.verifyBox}>
                  {verify.coverUrl ? (
                    <img className={classes.verifyCover} src={verify.coverUrl} alt="封面" />
                  ) : (
                    <Box className={classes.verifyCover} style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#888' }}>无封面</Box>
                  )}
                  <Box className={classes.verifyMain}>
                    <Field label="文件内嵌标题" value={verify.metadata?.title || '（空）'} />
                    <Field label="文件内嵌歌手" value={verify.metadata?.artist || '（空）'} />
                    <Field label="文件内嵌专辑" value={verify.metadata?.album || '（空）'} />
                    <Field label="年份" value={verify.metadata?.year || '-'} />
                    <Field label="音轨号" value={verify.metadata?.trackNumber || '-'} />
                    <Field label="文件路径" value={verify.task?.filePath || '-'} />
                    <Field label="下载用户" value={verify.userName} />
                  </Box>
                </Box>
              )}
              {verify.metadata?.lyrics && (
                <Box style={{ marginTop: 16 }}>
                  <Typography className={classes.fieldLabel}>内嵌歌词</Typography>
                  <Box className={classes.lyricsBox}>{verify.metadata.lyrics}</Box>
                </Box>
              )}
              <Box className={classes.editRow}>
                <TextField label="修正标题" size="small" variant="outlined" value={editTitle}
                  onChange={(e) => setEditTitle(e.target.value)} />
                <TextField label="修正歌手" size="small" variant="outlined" value={editArtist}
                  onChange={(e) => setEditArtist(e.target.value)} />
                <TextField label="修正专辑" size="small" variant="outlined" value={editAlbum}
                  onChange={(e) => setEditAlbum(e.target.value)} />
              </Box>
              <Typography style={{ fontSize: 12, color: '#888', marginTop: 8 }}>
                重新刮削会重新抓取封面/歌词并重写内嵌标签（不动音频流）；留空的字段保持当前值。
              </Typography>
            </>
          )}
          {rescraping && <LinearProgress style={{ marginTop: 12 }} />}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setVerify(null)}>关闭</Button>
          <Button color="primary" variant="contained" disabled={rescraping || !verify?.exists}
            onClick={handleRescrape} startIcon={<BuildIcon />}>
            {rescraping ? '刮削中…' : '重新刮削'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default DownloadManagePage