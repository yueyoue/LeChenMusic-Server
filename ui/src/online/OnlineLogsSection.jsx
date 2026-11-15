import React, { useState, useEffect, useCallback } from 'react'
import {
  Typography, Box, makeStyles, Button,
  Chip, TextField, MenuItem, Table, TableBody, TableCell,
  TableContainer, TableHead, TableRow, Paper, Tooltip, LinearProgress,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import RefreshIcon from '@material-ui/icons/Refresh'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「操作日志」区（仅管理员）：在线搜歌功能的操作审计日志。

const PAGE_SIZE = 50

const useStyles = makeStyles((theme) => ({
  filterRow: { display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', marginBottom: 16 },
  pathCell: { maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 12 },
  empty: { textAlign: 'center', padding: 48, color: theme.palette.text.secondary },
  pager: { display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 12, marginBottom: 24 },
  fieldLabel: { fontSize: 12, color: theme.palette.text.secondary },
  sectionTitle: { fontWeight: 700, marginBottom: 12, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 },
}))

const LOG_TYPES = [
  { value: '', label: '全部操作类型' },
  { value: 'download_queued', label: '加入下载' },
  { value: 'download_done', label: '下载完成' },
  { value: 'download_failed', label: '下载失败' },
  { value: 'delete', label: '删除歌曲' },
  { value: 'rescrape', label: '重新刮削' },
  { value: 'song_scrape', label: '歌曲刮削' },
]

const fmtTime = (v) => (v && !v.startsWith('0001-') ? new Date(v).toLocaleString() : '-')

const opLabel = (action) => {
  switch (action) {
    case 'download_queued': return '加入下载'
    case 'download_done': return '下载完成'
    case 'download_failed': return '下载失败'
    case 'delete': return '删除歌曲'
    case 'rescrape': return '重新刮削'
    case 'song_scrape': return '歌曲刮削'
    default: return action
  }
}

const OnlineLogsSection = () => {
  const classes = useStyles()
  const [logDraft, setLogDraft] = useState({ type: '', user: '', from: '', to: '' })
  const [logQuery, setLogQuery] = useState({ type: '', user: '', from: '', to: '', page: 1 })
  const [logs, setLogs] = useState([])
  const [logTotal, setLogTotal] = useState(0)
  const [logPages, setLogPages] = useState(0)
  const [logLoading, setLogLoading] = useState(true)
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')

  const fetchLogs = useCallback(async () => {
    setLogLoading(true)
    setError('')
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
    fetchLogs()
  }, [fetchLogs])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 4000)
    return () => clearTimeout(timer)
  }, [toast])

  const applyLogFilter = () => setLogQuery({ ...logDraft, page: 1 })

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

  return (
    <>
      <Typography className={classes.sectionTitle}>
        操作日志
        <Button size="small" startIcon={<RefreshIcon />} onClick={fetchLogs}>刷新</Button>
        <Button size="small" color="secondary" onClick={handleClearLogs}>清空日志</Button>
      </Typography>
      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}

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
                      <div className={classes.pathCell}>{l.detail || '-'}</div>
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
  )
}

export default OnlineLogsSection
