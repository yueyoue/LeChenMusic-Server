import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  CircularProgress, Chip, Divider
} from '@material-ui/core'
import SystemUpdateIcon from '@material-ui/icons/SystemUpdate'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import FileCopyIcon from '@material-ui/icons/FileCopy'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 600 },
  card: { marginBottom: 16 },
  header: { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 24 },
  versionInfo: { marginBottom: 16 },
  label: { fontSize: 12, color: theme.palette.text.secondary, marginBottom: 2 },
  value: { fontSize: 15, fontWeight: 600 },
  statusCard: {
    padding: '16px !important',
    display: 'flex', alignItems: 'center', gap: 12,
    borderRadius: 12,
  },
  statusOk: { backgroundColor: 'rgba(46, 213, 115, 0.1)', border: '1px solid rgba(46, 213, 115, 0.3)' },
  statusUpdate: { backgroundColor: 'rgba(255, 165, 2, 0.1)', border: '1px solid rgba(255, 165, 2, 0.3)' },
  statusError: { backgroundColor: 'rgba(255, 71, 87, 0.1)', border: '1px solid rgba(255, 71, 87, 0.3)' },
  commandBox: {
    backgroundColor: theme.palette.background.default,
    borderRadius: 8, padding: '12px 16px',
    fontFamily: 'monospace', fontSize: 13,
    wordBreak: 'break-all', position: 'relative',
    marginTop: 12,
  },
  copyBtn: {
    position: 'absolute', top: 8, right: 8,
    cursor: 'pointer', padding: 4,
    '&:hover': { opacity: 0.7 },
  },
  changelog: {
    fontSize: 13, color: theme.palette.text.secondary,
    lineHeight: 1.6, whiteSpace: 'pre-wrap',
    maxHeight: 200, overflow: 'auto',
    marginTop: 8, padding: 12,
    backgroundColor: theme.palette.background.default,
    borderRadius: 8,
  },
  checkBtn: { borderRadius: 20, textTransform: 'none', fontWeight: 600 },
}))

const VersionPage = () => {
  const classes = useStyles()
  const [loading, setLoading] = useState(true)
  const [checking, setChecking] = useState(false)
  const [versionInfo, setVersionInfo] = useState(null)
  const [updateInfo, setUpdateInfo] = useState(null)
  const [error, setError] = useState(null)
  const [copied, setCopied] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [updateLogs, setUpdateLogs] = useState([])
  const [restartCmd, setRestartCmd] = useState('')
  const [cmdCopied, setCmdCopied] = useState(false)

  useEffect(() => {
    fetchVersion()
  }, [])

  const fetchVersion = async () => {
    try {
      const token = localStorage.getItem('token')
      const headers = { 'X-ND-Authorization': `Bearer ${token}` }
      const res = await fetch('/api/version', { headers })
      if (res.ok) {
        const data = await res.json()
        setVersionInfo(data.data)
      }
    } catch (err) {
      console.error('Failed to fetch version:', err)
    } finally {
      setLoading(false)
    }
  }

  const checkForUpdate = async () => {
    setChecking(true)
    setError(null)
    try {
      const token = localStorage.getItem('token')
      const headers = { 'X-ND-Authorization': `Bearer ${token}` }
      const res = await fetch('/api/version/check', { headers })
      if (res.ok) {
        const data = await res.json()
        setUpdateInfo(data.data)
      } else {
        setError('检查更新失败')
      }
    } catch (err) {
      setError('网络错误，请稍后重试')
    } finally {
      setChecking(false)
    }
  }

  const copyCommand = () => {
    if (updateInfo?.updateCommand) {
      navigator.clipboard.writeText(updateInfo.updateCommand)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  const oneClickUpdate = async () => {
    setUpdating(true)
    setUpdateLogs(['🚀 开始一键更新...'])
    setRestartCmd('')
    try {
      const token = localStorage.getItem('token')
      const headers = { 'X-ND-Authorization': `Bearer ${token}` }
      const res = await fetch('/api/version/update', { method: 'POST', headers })
      const reader = res.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop()
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            const event = line.substring(7)
            const nextLine = lines[lines.indexOf(line) + 1]
            if (nextLine && nextLine.startsWith('data: ')) {
              const data = nextLine.substring(6)
              if (event === 'done') {
                setUpdateLogs(prev => [...prev, '✅ 镜像拉取完成！请手动重启容器。'])
                setUpdating(false)
                return
              }
              if (event === 'restart_cmd') {
                setRestartCmd(data)
                return
              }
              setUpdateLogs(prev => [...prev, data])
            }
          }
        }
      }
    } catch (err) {
      setUpdateLogs(prev => [...prev, '❌ 更新失败: ' + err.message])
    } finally {
      setUpdating(false)
    }
  }

  if (loading) {
    return <Box p={4} textAlign="center"><CircularProgress /></Box>
  }

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <SystemUpdateIcon style={{ fontSize: 28, color: 'primary.main' }} />
        <Typography variant="h6" style={{ fontWeight: 700 }}>版本更新</Typography>
      </Box>

      {/* Current Version */}
      <Card className={classes.card} elevation={0}>
        <CardContent>
          <Typography style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>当前版本</Typography>
          <Box className={classes.versionInfo}>
            <Typography className={classes.label}>版本号</Typography>
            <Typography className={classes.value}>
              {versionInfo?.currentSHA ? versionInfo.currentSHA.substring(0, 8) : '未知'}
            </Typography>
          </Box>
          <Box className={classes.versionInfo}>
            <Typography className={classes.label}>项目</Typography>
            <Typography className={classes.value}>{versionInfo?.serverName || 'LeChenMusic'}</Typography>
          </Box>
          <Box className={classes.versionInfo}>
            <Typography className={classes.label}>仓库</Typography>
            <Typography className={classes.value} style={{ fontSize: 13 }}>
              <a href={versionInfo?.serverUrl} target="_blank" rel="noopener" style={{ color: 'primary.main' }}>
                {versionInfo?.serverUrl}
              </a>
            </Typography>
          </Box>
        </CardContent>
      </Card>

      {/* Check Update Button */}
      <Box mb={2}>
        <Button
          variant="contained"
          color="primary"
          className={classes.checkBtn}
          onClick={checkForUpdate}
          disabled={checking}
          startIcon={checking ? <CircularProgress size={18} /> : <SystemUpdateIcon />}
        >
          {checking ? '检查中...' : '🔄 检查更新'}
        </Button>
      </Box>

      {/* Update Status */}
      {error && (
        <Card className={classes.card} elevation={0}>
          <CardContent className={`${classes.statusCard} ${classes.statusError}`}>
            <ErrorIcon style={{ color: '#ff4757', fontSize: 24 }} />
            <Typography style={{ color: '#ff4757' }}>{error}</Typography>
          </CardContent>
        </Card>
      )}

      {updateInfo && (
        <>
          {updateInfo.hasUpdate ? (
            <Card className={classes.card} elevation={0}>
              <CardContent className={`${classes.statusCard} ${classes.statusUpdate}`}>
                <SystemUpdateIcon style={{ color: '#ffa502', fontSize: 24 }} />
                <Box>
                  <Typography style={{ fontWeight: 600, color: '#ffa502' }}>发现新版本！</Typography>
                  <Typography style={{ fontSize: 12, color: 'text.secondary', marginTop: 4 }}>
                    最新版本: {updateInfo.latestTag} · {updateInfo.latestDate ? new Date(updateInfo.latestDate).toLocaleDateString() : ''}
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          ) : (
            <Card className={classes.card} elevation={0}>
              <CardContent className={`${classes.statusCard} ${classes.statusOk}`}>
                <CheckCircleIcon style={{ color: '#2ed573', fontSize: 24 }} />
                <Typography style={{ fontWeight: 600, color: '#2ed573' }}>已是最新版本</Typography>
              </CardContent>
            </Card>
          )}

          {/* Changelog */}
          {updateInfo.changelog && (
            <Card className={classes.card} elevation={0}>
              <CardContent>
                <Typography style={{ fontSize: 14, fontWeight: 600, marginBottom: 8 }}>更新内容</Typography>
                <Box className={classes.changelog}>{updateInfo.changelog}</Box>
              </CardContent>
            </Card>
          )}

          {/* One-Click Update + Command */}
          {updateInfo.hasUpdate && (
            <Card className={classes.card} elevation={0}>
              <CardContent>
                <Typography style={{ fontSize: 14, fontWeight: 600, marginBottom: 12 }}>更新方式</Typography>
                
                {/* One-click update button */}
                <Button
                  variant="contained"
                  color="secondary"
                  style={{ borderRadius: 20, textTransform: 'none', fontWeight: 600, marginBottom: 16 }}
                  onClick={oneClickUpdate}
                  disabled={updating}
                  startIcon={updating ? <CircularProgress size={18} /> : <SystemUpdateIcon />}
                >
                  {updating ? '更新中...' : '🚀 一键更新'}
                </Button>

                {/* Update logs */}
                {updateLogs.length > 0 && (
                  <Box className={classes.commandBox} style={{ maxHeight: 200, overflow: 'auto', marginTop: 8 }}>
                    {updateLogs.map((log, i) => (
                      <Typography key={i} style={{ fontFamily: 'monospace', fontSize: 12, lineHeight: 1.8 }}>
                        {log}
                      </Typography>
                    ))}
                  </Box>
                )}

                {/* Restart command (shown after pull completes) */}
                {restartCmd && (
                  <Box mt={2} style={{ backgroundColor: 'rgba(255, 165, 2, 0.08)', borderRadius: 12, padding: 16, border: '1px solid rgba(255, 165, 2, 0.3)' }}>
                    <Typography style={{ fontSize: 14, fontWeight: 600, marginBottom: 8, color: '#ffa502' }}>
                      ⚠️ 镜像已就绪，请在服务器上执行以下命令重启：
                    </Typography>
                    <Box className={classes.commandBox}>
                      <Typography style={{ fontFamily: 'monospace', fontSize: 12, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
                        {restartCmd}
                      </Typography>
                      <Box className={classes.copyBtn} onClick={() => {
                        navigator.clipboard.writeText(restartCmd)
                        setCmdCopied(true)
                        setTimeout(() => setCmdCopied(false), 2000)
                      }}>
                        <FileCopyIcon style={{ fontSize: 16, color: 'text.secondary' }} />
                      </Box>
                    </Box>
                    {cmdCopied && (
                      <Typography style={{ fontSize: 12, color: '#2ed573', marginTop: 8 }}>
                        ✅ 已复制到剪贴板
                      </Typography>
                    )}
                  </Box>
                )}

                {/* Manual command fallback */}
                {!restartCmd && (
                  <Box mt={2}>
                    <Typography style={{ fontSize: 12, color: 'text.secondary', marginBottom: 8 }}>
                      或手动在服务器上执行：
                    </Typography>
                    <Box className={classes.commandBox}>
                      <Typography style={{ fontFamily: 'monospace', fontSize: 13 }}>
                        {updateInfo.updateCommand}
                      </Typography>
                      <Box className={classes.copyBtn} onClick={copyCommand}>
                        <FileCopyIcon style={{ fontSize: 16, color: 'text.secondary' }} />
                      </Box>
                    </Box>
                    {copied && (
                      <Typography style={{ fontSize: 12, color: '#2ed573', marginTop: 8 }}>
                        ✅ 已复制到剪贴板
                      </Typography>
                    )}
                  </Box>
                )}
              </CardContent>
            </Card>
          )}
        </>
      )}
    </Box>
  )
}

export default VersionPage

