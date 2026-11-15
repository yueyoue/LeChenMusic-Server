import React, { useState, useEffect, useCallback } from 'react'
import {
  Typography, Box, makeStyles, Button, IconButton,
  Tooltip, Chip, TextField, MenuItem, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Dialog, DialogTitle,
  DialogContent, DialogActions, LinearProgress,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import RefreshIcon from '@material-ui/icons/Refresh'
import DeleteIcon from '@material-ui/icons/Delete'
import BuildIcon from '@material-ui/icons/Build'
import VisibilityIcon from '@material-ui/icons/Visibility'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'
import { confirmDeleteText, deleteResultText } from './downloadDelete'

// 「下载管理」区（仅管理员）：下载记录、刮削验证、重新刮削、删除下载歌曲。

const PAGE_SIZE = 50

const useStyles = makeStyles((theme) => ({
  filterRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginBottom: 16 },
  tableWrap: { marginBottom: 12 },
  pathCell: { maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 12, fontFamily: 'monospace' },
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
  sectionTitle: { fontWeight: 700, marginBottom: 12, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 },
}))

const SCRAPE_FILTERS = [
  { value: '', label: '全部刮削状态' },
  { value: 'ok', label: '刮削完整' },
  { value: 'missing-cover', label: '缺封面' },
  { value: 'missing-lyrics', label: '缺歌词' },
  { value: 'missing-tags', label: '缺标签' },
]

const fmtTime = (v) => (v && !v.startsWith('0001-') ? new Date(v).toLocaleString() : '-')

const Field = ({ label, value }) => {
  const classes = useStyles()
  return (
    <Box className={classes.fieldRow}>
      <Typography className={classes.fieldLabel}>{label}</Typography>
      <Typography className={classes.fieldValue}>{value}</Typography>
    </Box>
  )
}

const OnlineDownloadsSection = () => {
  const classes = useStyles()
  const [dlDraft, setDlDraft] = useState({ q: '', user: '', scrape: '' })
  const [dlQuery, setDlQuery] = useState({ q: '', user: '', scrape: '', page: 1 })
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [verify, setVerify] = useState(null)
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

  useEffect(() => {
    fetchDownloads()
  }, [fetchDownloads])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 4000)
    return () => clearTimeout(timer)
  }, [toast])

  const applyDlFilter = () => setDlQuery({ ...dlDraft, page: 1 })

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
    if (!window.confirm(confirmDeleteText(item))) return
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/downloads/${item.id}`, { method: 'DELETE' })
      setToast(deleteResultText(res.json, item))
      fetchDownloads()
    } catch (e) {
      setError('删除失败：' + (e.body || e.message || ''))
    }
  }

  const handleRescrape = async () => {
    if (!verify) return
    setRescraping(true)
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/downloads/${verify.task.id}/rescrape`, {
        method: 'POST',
        body: JSON.stringify({ title: editTitle, artist: editArtist, album: editAlbum }),
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
    <>
      <Typography className={classes.sectionTitle}>
        下载记录
        <Button size="small" startIcon={<RefreshIcon />} onClick={fetchDownloads}>刷新</Button>
      </Typography>
      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}

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
          <Typography>暂无下载记录</Typography>
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
          <Typography className={classes.fieldLabel}>第 {dlQuery.page} / {dlPages} 页</Typography>
          <Button size="small" disabled={dlQuery.page <= 1}
            onClick={() => setDlQuery({ ...dlQuery, page: dlQuery.page - 1 })}>上一页</Button>
          <Button size="small" disabled={dlQuery.page >= dlPages}
            onClick={() => setDlQuery({ ...dlQuery, page: dlQuery.page + 1 })}>下一页</Button>
        </Box>
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
                    <Field label="专辑艺术家" value={verify.metadata?.albumArtist || '-'} />
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
                标题会自动清洗（如「相思风雨中-《出位江湖》电视剧插曲」→「相思风雨中」），多歌手自动按「A • B」格式入库。
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
    </>
  )
}

export default OnlineDownloadsSection
