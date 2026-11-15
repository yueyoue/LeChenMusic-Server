import React, { useState, useEffect, useCallback, useRef } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  Chip, CircularProgress, LinearProgress,
} from '@material-ui/core'
import RefreshIcon from '@material-ui/icons/Refresh'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import { Alert } from '@material-ui/lab'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'
import { confirmDeleteText, deleteResultText } from './downloadDelete'

// 「下载任务」区：队列/进行中/完成/失败的任务，3s 轮询进度。

const useStyles = makeStyles((theme) => ({
  sectionTitle: { fontWeight: 700, marginBottom: 12, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 },
  taskCard: { marginBottom: 8, borderLeft: '4px solid #1e90ff' },
  taskDone: { borderLeftColor: '#2ed573' },
  taskFailed: { borderLeftColor: '#ff4757' },
  taskRow: { display: 'flex', alignItems: 'center', gap: 8 },
  taskMain: { flex: 1, minWidth: 0 },
  songName: { fontWeight: 600, fontSize: 14 },
  sub: { fontSize: 12, color: theme.palette.text.secondary },
  progressRow: { display: 'flex', alignItems: 'center', gap: 8, marginTop: 8 },
  empty: { textAlign: 'center', padding: 40, color: theme.palette.text.secondary },
}))

const ACTIVE_STATUSES = ['queued', 'resolving', 'downloading', 'tagging']

const fmtSize = (bytes) => {
  if (!bytes || bytes <= 0) return ''
  const mb = bytes / 1024 / 1024
  return mb >= 1024 ? `${(mb / 1024).toFixed(2)} GB` : `${mb.toFixed(1)} MB`
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

const OnlineTasksSection = ({ onActiveCount }) => {
  const classes = useStyles()
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
      const active = list.filter((t) => ACTIVE_STATUSES.includes(t.status)).length
      if (onActiveCount) onActiveCount(active)
      // 首次轮询只做基线，不弹历史任务的完成提示
      const announce = !firstPoll.current
      firstPoll.current = false
      list.forEach((t) => {
        if (t.status !== 'done' || doneIds.current.has(t.id)) return
        doneIds.current.add(t.id)
        if (announce) setToast(`《${t.songName}》已入库，可播放`)
      })
    } catch (e) {
      // 轮询失败静默，下一轮重试
    }
  }, [onActiveCount])

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

  const handleRetry = async (task) => {
    try {
      await httpClient(`${REST_URL}/online/tasks/${task.id}/retry`, { method: 'POST' })
      fetchTasks()
    } catch (e) {
      setError('重试失败：' + (e.message || ''))
    }
  }

  const handleDelete = async (task) => {
    // 删除会连 NAS 上的音频文件一起删掉，必须先确认
    if (!window.confirm(confirmDeleteText(task))) return
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/tasks/${task.id}`, { method: 'DELETE' })
      setToast(deleteResultText(res.json, task))
      doneIds.current.delete(task.id)
      fetchTasks()
    } catch (e) {
      setError('删除失败：' + (e.body || e.message || ''))
    }
  }

  const activeTasks = tasks.filter((t) => ACTIVE_STATUSES.includes(t.status))
  const finishedTasks = tasks.filter((t) => t.status === 'done')
  const failedTasks = tasks.filter((t) => t.status === 'failed')

  return (
    <>
      <Typography className={classes.sectionTitle}>
        下载任务
        <Button size="small" startIcon={<RefreshIcon />} onClick={fetchTasks}>刷新</Button>
      </Typography>
      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}

      {tasks.length === 0 ? (
        <Box className={classes.empty}>
          <Typography>暂无下载任务，去「搜歌下载」搜一首吧</Typography>
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
                  <Button size="small" color="secondary" onClick={() => handleDelete(t)}>删除</Button>
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
                  <Button size="small" color="secondary" onClick={() => handleDelete(t)}>删除</Button>
                </Box>
              </CardContent>
            </Card>
          ))}
        </>
      )}
    </>
  )
}

export default OnlineTasksSection
