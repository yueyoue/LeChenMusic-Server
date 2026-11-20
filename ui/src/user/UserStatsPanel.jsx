import React, { useState, useEffect } from 'react'
import { makeStyles } from '@material-ui/core/styles'
import {
  Typography,
  Box,
  Chip,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from '@material-ui/core'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const useStyles = makeStyles((theme) => ({
  section: {
    marginTop: theme.spacing(2),
    padding: theme.spacing(2),
    backgroundColor: theme.palette.background.default,
  },
  sectionTitle: {
    marginBottom: theme.spacing(1),
    fontWeight: 600,
  },
  deviceLine: {
    display: 'flex',
    alignItems: 'center',
    marginBottom: theme.spacing(0.5),
  },
  deviceInfo: {
    marginLeft: theme.spacing(1),
    color: theme.palette.text.secondary,
    fontSize: 13,
  },
  empty: {
    color: theme.palette.text.secondary,
  },
}))

const formatDuration = (seconds) => {
  if (!seconds || seconds <= 0) return '-'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}小时${m}分钟`
  if (m > 0) return `${m}分${s}秒`
  return `${s}秒`
}

const formatTime = (ts) => (ts ? new Date(ts).toLocaleString() : '-')

// 把相邻的同一曲目片段合并成一条，读起来更像“最近播放了什么”（模糊统计，不做精确去重）
const mergeAdjacent = (logs) => {
  const result = []
  ;(logs || []).forEach((log) => {
    const last = result[result.length - 1]
    if (last && last.itemType === log.itemType && last.itemId === log.itemId) {
      last.duration += log.duration || 0
      if (log.playDate > last.playDate) last.playDate = log.playDate
    } else {
      result.push({ ...log })
    }
  })
  return result
}

const UserStatsPanel = ({ userId }) => {
  const classes = useStyles()
  const [devices, setDevices] = useState(null)
  const [playLogs, setPlayLogs] = useState(null)

  useEffect(() => {
    if (!userId) return
    setDevices(null)
    setPlayLogs(null)
    httpClient(`${REST_URL}/stats/users/${userId}/devices`)
      .then((res) => setDevices(res.json?.data || []))
      .catch(() => setDevices([]))
    httpClient(`${REST_URL}/stats/users/${userId}/play-logs?limit=50`)
      .then((res) => setPlayLogs(mergeAdjacent(res.json?.data || [])))
      .catch(() => setPlayLogs([]))
  }, [userId])

  return (
    <Paper className={classes.section} variant="outlined">
      <Typography variant="subtitle1" className={classes.sectionTitle}>
        手机端版本
      </Typography>
      {devices === null ? (
        <Typography variant="body2" className={classes.empty}>
          加载中…
        </Typography>
      ) : devices.length === 0 ? (
        <Typography variant="body2" className={classes.empty}>
          暂无上报（旧版 APP 不会上报版本信息，用户升级后自动显示）
        </Typography>
      ) : (
        devices.map((d) => (
          <div key={d.id} className={classes.deviceLine}>
            <Chip label={d.appVersion || '-'} size="small" variant="outlined" color="primary" />
            <span className={classes.deviceInfo}>
              {d.deviceInfo || ''} · 最后活跃 {formatTime(d.lastSeenAt)}
            </span>
          </div>
        ))
      )}

      <Typography variant="subtitle1" className={classes.sectionTitle} style={{ marginTop: 16 }}>
        最近播放记录（音乐 / 有声书）
      </Typography>
      {playLogs === null ? (
        <Typography variant="body2" className={classes.empty}>
          加载中…
        </Typography>
      ) : playLogs.length === 0 ? (
        <Typography variant="body2" className={classes.empty}>
          暂无播放记录
        </Typography>
      ) : (
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>播放时间</TableCell>
                <TableCell>类型</TableCell>
                <TableCell>标题</TableCell>
                <TableCell>歌手 / 作者</TableCell>
                <TableCell align="right">收听时长</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {playLogs.map((log, idx) => (
                <TableRow key={log.id || idx} hover>
                  <TableCell style={{ whiteSpace: 'nowrap' }}>{formatTime(log.playDate)}</TableCell>
                  <TableCell>
                    <Chip
                      label={log.itemType === 'audiobook' ? '有声书' : '音乐'}
                      size="small"
                      variant="outlined"
                      color={log.itemType === 'audiobook' ? 'secondary' : 'primary'}
                    />
                  </TableCell>
                  <TableCell>{log.itemTitle || log.itemId}</TableCell>
                  <TableCell>{log.itemArtist || '-'}</TableCell>
                  <TableCell align="right">{formatDuration(log.duration)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Paper>
  )
}

export default UserStatsPanel
