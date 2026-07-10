import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  CircularProgress, Chip
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
  stepBox: {
    backgroundColor: 'rgba(25, 118, 210, 0.06)',
    borderRadius: 12, padding: 16, marginTop: 12,
    border: '1px solid rgba(25, 118, 210, 0.15)',
  },
  step: {
    display: 'flex', alignItems: 'flex-start', gap: 8, marginBottom: 8,
    '&:last-child': { marginBottom: 0 },
  },
  stepNum: {
    width: 20, height: 20, borderRadius: 10,
    backgroundColor: theme.palette.primary.main,
    color: '#fff', fontSize: 11, fontWeight: 700,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    flexShrink: 0, marginTop: 2,
  },
  stepText: { fontSize: 13, lineHeight: 1.6 },
}))

const VersionPage = () => {
  const classes = useStyles()
  const [loading, setLoading] = useState(true)
  const [checking, setChecking] = useState(false)
  const [versionInfo, setVersionInfo] = useState(null)
  const [updateInfo, setUpdateInfo] = useState(null)
  const [error, setError] = useState(null)
  const [cmdCopied, setCmdCopied] = useState(false)

  useEffect(() => { fetchVersion() }, [])

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
      setCmdCopied(true)
      setTimeout(() => setCmdCopied(false), 2000)
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
              {versionInfo?.currentSHA ? versionInfo.currentSHA.substring(0, 7) : '未知'}
            </Typography>
          </Box>
          <Box className={classes.versionInfo}>
            <Typography className={classes.label}>项目</Typography>
            <Typography className={classes.value}>{versionInfo?.serverName || 'LeChenMusic'}</Typography>
          </Box>
          <Box className={classes.versionInfo}>
            <Typography className={classes.label}>仓库</Typography>
            <Typography className={classes.value} style={{ fontSize: 13 }}>
              <a href={versionInfo?.serverUrl} target="_blank" rel="noopener" style={{ color: '#1976d2' }}>
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

      {/* Error */}
      {error && (
        <Card className={classes.card} elevation={0}>
          <CardContent className={`${classes.statusCard} ${classes.statusError}`}>
            <ErrorIcon style={{ color: '#ff4757', fontSize: 24 }} />
            <Typography style={{ color: '#ff4757' }}>{error}</Typography>
          </CardContent>
        </Card>
      )}

      {/* Update Status */}
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
                <Typography style={{ fontWeight: 600, color: '#2ed573' }}>已是最新版本 ✓</Typography>
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

          {/* Update Command */}
          {updateInfo.hasUpdate && (
            <Card className={classes.card} elevation={0}>
              <CardContent>
                <Typography style={{ fontSize: 14, fontWeight: 600, marginBottom: 8 }}>SSH 更新命令</Typography>
                <Typography style={{ fontSize: 12, color: 'text.secondary', marginBottom: 12 }}>
                  登录服务器，进入 docker-compose.yml 所在目录，执行以下命令：
                </Typography>

                <Box className={classes.stepBox}>
                  <Box className={classes.step}>
                    <Box className={classes.stepNum}>1</Box>
                    <Typography className={classes.stepText}>SSH 登录服务器</Typography>
                  </Box>
                  <Box className={classes.step}>
                    <Box className={classes.stepNum}>2</Box>
                    <Typography className={classes.stepText}>
                      进入安装目录：<code style={{ background: 'rgba(0,0,0,0.06)', padding: '2px 6px', borderRadius: 4 }}>
                        cd /opt/lechenmusic
                      </code>
                    </Typography>
                  </Box>
                  <Box className={classes.step}>
                    <Box className={classes.stepNum}>3</Box>
                    <Typography className={classes.stepText}>执行更新命令：</Typography>
                  </Box>
                </Box>

                <Box className={classes.commandBox}>
                  <Typography style={{ fontFamily: 'monospace', fontSize: 13, lineHeight: 1.6 }}>
                    {updateInfo.updateCommand}
                  </Typography>
                  <Box className={classes.copyBtn} onClick={copyCommand}>
                    <FileCopyIcon style={{ fontSize: 16, color: 'text.secondary' }} />
                  </Box>
                </Box>

                {cmdCopied && (
                  <Typography style={{ fontSize: 12, color: '#2ed573', marginTop: 8 }}>
                    ✅ 已复制到剪贴板
                  </Typography>
                )}

                <Box mt={2} p={1.5} style={{ backgroundColor: 'rgba(255,165,2,0.06)', borderRadius: 8 }}>
                  <Typography style={{ fontSize: 11, color: 'text.secondary' }}>
                    💡 提示：如果你的安装目录不是 <code>/opt/lechenmusic</code>，请替换为实际路径。
                    命令会自动拉取最新镜像并重建容器，数据不会丢失。
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </Box>
  )
}

export default VersionPage
