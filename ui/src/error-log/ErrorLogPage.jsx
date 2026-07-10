import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  IconButton, Tooltip, Chip, Divider, Dialog, DialogTitle,
  DialogContent, DialogActions
} from '@material-ui/core'
import ErrorIcon from '@material-ui/icons/Error'
import DeleteSweepIcon from '@material-ui/icons/DeleteSweep'
import RefreshIcon from '@material-ui/icons/Refresh'
import BugReportIcon from '@material-ui/icons/BugReport'
import PhoneAndroidIcon from '@material-ui/icons/PhoneAndroid'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1000, margin: '0 auto' },
  header: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 },
  headerLeft: { display: 'flex', alignItems: 'center', gap: 12 },
  logCard: { marginBottom: 8, borderLeft: '4px solid #ff4757' },
  logCardWarning: { borderLeftColor: '#ffa502' },
  logHeader: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 },
  logMeta: { display: 'flex', gap: 8, flexWrap: 'wrap' },
  logMessage: { fontSize: 14, fontWeight: 500, marginBottom: 4 },
  logStack: { fontSize: 12, color: theme.palette.text.secondary, fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-all', maxHeight: 150, overflow: 'auto' },
  empty: { textAlign: 'center', padding: 60, color: theme.palette.text.secondary },
  statChip: { marginRight: 8 },
}))

const ErrorLogPage = () => {
  const classes = useStyles()
  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedLog, setSelectedLog] = useState(null)

  const fetchLogs = async () => {
    setLoading(true)
    try {
      const res = await httpClient(`${REST_URL}/error-log`)
      setLogs(res.json?.data || [])
    } catch (e) {
      console.error('Failed to fetch error logs:', e)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchLogs() }, [])

  const handleClear = async () => {
    if (!window.confirm('确定清空所有错误日志？')) return
    try {
      await httpClient(`${REST_URL}/error-log`, { method: 'DELETE' })
      setLogs([])
    } catch (e) {
      console.error('Failed to clear logs:', e)
    }
  }

  const errorCount = logs.filter(l => l.level === 'error' || l.level === 'crash').length
  const warnCount = logs.filter(l => l.level === 'warning').length

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <Box className={classes.headerLeft}>
          <BugReportIcon style={{ fontSize: 28, color: '#ff4757' }} />
          <Typography variant="h6" style={{ fontWeight: 700 }}>APP 错误日志</Typography>
          <Chip label={`${errorCount} 错误`} size="small" color="secondary" className={classes.statChip} />
          <Chip label={`${warnCount} 警告`} size="small" style={{ background: '#ffa502', color: '#fff' }} />
        </Box>
        <Box>
          <Tooltip title="刷新">
            <IconButton onClick={fetchLogs}><RefreshIcon /></IconButton>
          </Tooltip>
          <Tooltip title="清空日志">
            <IconButton onClick={handleClear}><DeleteSweepIcon /></IconButton>
          </Tooltip>
        </Box>
      </Box>

      {loading ? (
        <Box textAlign="center" py={4}><Typography>加载中...</Typography></Box>
      ) : logs.length === 0 ? (
        <Box className={classes.empty}>
          <BugReportIcon style={{ fontSize: 64, opacity: 0.2 }} />
          <Typography style={{ marginTop: 16, fontSize: 16 }}>暂无错误日志</Typography>
          <Typography style={{ marginTop: 8, fontSize: 13, color: 'text.secondary' }}>
            APP 端发生的错误和崩溃会自动上报到这里
          </Typography>
        </Box>
      ) : (
        logs.map((log, idx) => (
          <Card
            key={log.id || idx}
            className={`${classes.logCard} ${log.level === 'warning' ? classes.logCardWarning : ''}`}
            elevation={1}
            style={{ cursor: 'pointer' }}
            onClick={() => setSelectedLog(log)}
          >
            <CardContent style={{ padding: '12px 16px' }}>
              <Box className={classes.logHeader}>
                <Typography className={classes.logMessage}>
                  {log.level === 'crash' ? '💥' : log.level === 'error' ? '❌' : '⚠️'}{' '}
                  {log.message?.substring(0, 100)}
                </Typography>
                <Typography style={{ fontSize: 11, color: 'text.secondary' }}>
                  {log.timestamp ? new Date(log.timestamp).toLocaleString() : ''}
                </Typography>
              </Box>
              <Box className={classes.logMeta}>
                {log.screen && <Chip label={log.screen} size="small" variant="outlined" />}
                {log.device && <Chip label={log.device} size="small" variant="outlined" icon={<PhoneAndroidIcon />} />}
                {log.appVersion && <Chip label={`v${log.appVersion}`} size="small" variant="outlined" />}
                {log.os && <Chip label={log.os} size="small" variant="outlined" />}
              </Box>
            </CardContent>
          </Card>
        ))
      )}

      {/* Detail Dialog */}
      <Dialog open={!!selectedLog} onClose={() => setSelectedLog(null)} maxWidth="md" fullWidth>
        <DialogTitle>
          {selectedLog?.level === 'crash' ? '💥 崩溃详情' : selectedLog?.level === 'error' ? '❌ 错误详情' : '⚠️ 警告详情'}
        </DialogTitle>
        <DialogContent>
          {selectedLog && (
            <Box>
              <Typography style={{ fontWeight: 600, marginBottom: 8 }}>{selectedLog.message}</Typography>
              <Box className={classes.logMeta} style={{ marginBottom: 12 }}>
                {selectedLog.screen && <Chip label={`页面: ${selectedLog.screen}`} size="small" />}
                {selectedLog.device && <Chip label={`设备: ${selectedLog.device}`} size="small" />}
                {selectedLog.os && <Chip label={`系统: ${selectedLog.os}`} size="small" />}
                {selectedLog.appVersion && <Chip label={`版本: ${selectedLog.appVersion}`} size="small" />}
                {selectedLog.userId && <Chip label={`用户: ${selectedLog.userId}`} size="small" />}
              </Box>
              <Typography style={{ fontSize: 12, color: 'text.secondary', marginBottom: 4 }}>时间</Typography>
              <Typography style={{ fontSize: 13, marginBottom: 12 }}>{selectedLog.timestamp}</Typography>
              {selectedLog.stack && (
                <>
                  <Typography style={{ fontSize: 12, color: 'text.secondary', marginBottom: 4 }}>堆栈信息</Typography>
                  <Box style={{ background: '#f5f5f5', borderRadius: 8, padding: 12 }}>
                    <Typography style={{ fontSize: 12, fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                      {selectedLog.stack}
                    </Typography>
                  </Box>
                </>
              )}
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSelectedLog(null)}>关闭</Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default ErrorLogPage
