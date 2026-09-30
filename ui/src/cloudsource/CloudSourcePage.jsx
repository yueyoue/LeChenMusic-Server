import React, { useState, useEffect, useCallback } from 'react'
import {
  Typography,
  Box,
  Card,
  CardContent,
  makeStyles,
  Button,
  IconButton,
  Tooltip,
  Chip,
  Switch,
  FormControlLabel,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  CircularProgress,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import CloudIcon from '@material-ui/icons/Cloud'
import RefreshIcon from '@material-ui/icons/Refresh'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import HelpIcon from '@material-ui/icons/Help'
import PauseCircleIcon from '@material-ui/icons/PauseCircle'
import SyncIcon from '@material-ui/icons/Sync'
import { useSelector } from 'react-redux'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'
import subsonic from '../subsonic'

// 云源管理面板 (评审 P2-4)：网关状态 / 文件数 / 最后扫描时间 / 单源扫描 / 启停。
// 启停语义：停用的网关会被所有扫描跳过（定时 + 手动），已入库媒体仍可播放。

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1000, margin: '0 auto' },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: 16,
  },
  headerLeft: { display: 'flex', alignItems: 'center', gap: 12 },
  gatewayCard: { marginBottom: 16 },
  gatewayHeader: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    flexWrap: 'wrap',
    gap: 8,
  },
  gatewayTitle: { display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' },
  gatewayUrl: { color: theme.palette.text.secondary, fontSize: 13, marginTop: 4 },
  statsRow: { display: 'flex', gap: 24, flexWrap: 'wrap', marginTop: 12 },
  stat: { textAlign: 'center' },
  statValue: { fontSize: 20, fontWeight: 700 },
  statLabel: { fontSize: 12, color: theme.palette.text.secondary },
  actions: { display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' },
  empty: { textAlign: 'center', padding: 60, color: theme.palette.text.secondary },
  checkMeta: { fontSize: 12, color: theme.palette.text.secondary, marginTop: 8 },
  alertBox: { marginBottom: 8 },
}))

const fmtTime = (value) => {
  if (!value || value.startsWith('0001-')) return '从未'
  return new Date(value).toLocaleString()
}

const statusChip = (gw, classes) => {
  if (!gw.enabled) {
    return (
      <Chip
        size="small"
        icon={<PauseCircleIcon />}
        label="已停用"
        style={{ background: '#dfe4ea', color: '#2f3542' }}
      />
    )
  }
  if (!gw.check) {
    return <Chip size="small" icon={<HelpIcon />} label="未检测" variant="outlined" />
  }
  if (gw.check.ok) {
    return (
      <Chip
        size="small"
        icon={<CheckCircleIcon />}
        label="正常"
        style={{ background: '#2ed573', color: '#fff' }}
      />
    )
  }
  return (
    <Chip
      size="small"
      icon={<ErrorIcon />}
      label="异常"
      style={{ background: '#ff4757', color: '#fff' }}
    />
  )
}

const CloudSourcePage = () => {
  const classes = useStyles()
  const [gateways, setGateways] = useState([])
  const [loading, setLoading] = useState(true)
  const [checking, setChecking] = useState(null)
  const [toggling, setToggling] = useState(null)
  const [scanning, setScanning] = useState(null)
  const scanStatus = useSelector((state) => state.activity.scanStatus)

  const loadGateways = useCallback(async () => {
    setLoading(true)
    try {
      const res = await httpClient(`${REST_URL}/cloudsource/gateways`)
      setGateways(res.json?.data?.gateways || [])
    } catch (e) {
      console.error('Failed to fetch cloud gateways:', e)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadGateways()
  }, [loadGateways])

  const handleCheck = async (name) => {
    setChecking(name)
    try {
      const res = await httpClient(
        `${REST_URL}/cloudsource/gateways/${encodeURIComponent(name)}/check`,
        { method: 'POST' },
      )
      const check = res.json?.data
      setGateways((prev) =>
        prev.map((gw) => (gw.name === name ? { ...gw, check } : gw)),
      )
    } catch (e) {
      console.error('Gateway check failed:', e)
    } finally {
      setChecking(null)
    }
  }

  const handleToggle = async (name, enabled) => {
    setToggling(name)
    try {
      await httpClient(
        `${REST_URL}/cloudsource/gateways/${encodeURIComponent(name)}/enabled`,
        {
          method: 'PUT',
          body: JSON.stringify({ enabled }),
        },
      )
      setGateways((prev) =>
        prev.map((gw) => (gw.name === name ? { ...gw, enabled } : gw)),
      )
    } catch (e) {
      console.error('Gateway toggle failed:', e)
    } finally {
      setToggling(null)
    }
  }

  // 单源扫描：复用现有 startScan 通道，target = 该网关绑定的库
  const handleScan = async (gw) => {
    const targets = (gw.libraries || []).map((lib) => `${lib.id}:`)
    if (targets.length === 0) return
    setScanning(gw.name)
    try {
      await subsonic.startScan({ target: targets })
    } catch (e) {
      console.error('Gateway scan failed:', e)
    } finally {
      setScanning(null)
    }
  }

  const warnings = gateways.filter((gw) => (gw.check && !gw.check.ok) || !gw.enabled)

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <Box className={classes.headerLeft}>
          <CloudIcon style={{ fontSize: 28, color: '#1e90ff' }} />
          <Typography variant="h6" style={{ fontWeight: 700 }}>
            云源管理
          </Typography>
          <Chip label={`${gateways.length} 个网关`} size="small" />
        </Box>
        <Tooltip title="刷新">
          <IconButton onClick={loadGateways}>
            <RefreshIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {warnings.map((gw) => (
        <Alert
          key={`warn-${gw.name}`}
          className={classes.alertBox}
          severity={!gw.enabled ? 'warning' : 'error'}
        >
          {!gw.enabled
            ? `网关「${gw.name}」已停用：所有扫描将跳过它的云库（已入库媒体仍可播放）`
            : `网关「${gw.name}」连接异常：${gw.check?.error || '未知错误'}`}
        </Alert>
      ))}

      {loading ? (
        <Box textAlign="center" py={4}>
          <CircularProgress />
        </Box>
      ) : gateways.length === 0 ? (
        <Box className={classes.empty}>
          <CloudIcon style={{ fontSize: 64, opacity: 0.2 }} />
          <Typography style={{ marginTop: 16, fontSize: 16 }}>
            未配置任何 OpenList 网关
          </Typography>
          <Typography style={{ marginTop: 8, fontSize: 13 }}>
            在服务端 navidrome.toml 中添加 [OpenList.&lt;名称&gt;] 配置后即可在此管理
          </Typography>
        </Box>
      ) : (
        gateways.map((gw) => (
          <Card key={gw.name} className={classes.gatewayCard} elevation={2}>
            <CardContent>
              <Box className={classes.gatewayHeader}>
                <Box>
                  <Box className={classes.gatewayTitle}>
                    <Typography variant="h6" style={{ fontWeight: 700 }}>
                      {gw.name}
                    </Typography>
                    {statusChip(gw, classes)}
                    <Chip size="small" label={gw.host} variant="outlined" />
                  </Box>
                  <Typography className={classes.gatewayUrl}>{gw.url}</Typography>
                </Box>
                <FormControlLabel
                  control={
                    <Switch
                      checked={gw.enabled}
                      disabled={toggling === gw.name}
                      onChange={(e) => handleToggle(gw.name, e.target.checked)}
                      color="primary"
                    />
                  }
                  label={gw.enabled ? '启用中' : '已停用'}
                />
              </Box>

              <Box className={classes.statsRow}>
                <Box className={classes.stat}>
                  <Typography className={classes.statValue}>{gw.totalFiles}</Typography>
                  <Typography className={classes.statLabel}>文件数</Typography>
                </Box>
                <Box className={classes.stat}>
                  <Typography className={classes.statValue}>{gw.totalSongs}</Typography>
                  <Typography className={classes.statLabel}>歌曲数</Typography>
                </Box>
                <Box className={classes.stat}>
                  <Typography className={classes.statValue} style={{ fontSize: 15 }}>
                    {fmtTime(gw.lastScanAt)}
                  </Typography>
                  <Typography className={classes.statLabel}>最后扫描</Typography>
                </Box>
                <Box className={classes.stat}>
                  <Typography className={classes.statValue} style={{ fontSize: 15 }}>
                    {gw.check ? fmtTime(gw.check.checkedAt) : '未检测'}
                  </Typography>
                  <Typography className={classes.statLabel}>连通性检测</Typography>
                </Box>
              </Box>

              {gw.check && (
                <Typography className={classes.checkMeta}>
                  最近检测（{gw.check.source === 'startup' ? '启动自检' : '手动'}）：
                  {gw.check.ok
                    ? ` 正常，根目录 ${gw.check.rootEntries} 项`
                    : ` 失败 — ${gw.check.error}`}
                </Typography>
              )}

              <Box className={classes.actions}>
                <Button
                  variant="outlined"
                  color="primary"
                  disabled={checking === gw.name}
                  startIcon={
                    checking === gw.name ? (
                      <CircularProgress size={16} />
                    ) : (
                      <CheckCircleIcon />
                    )
                  }
                  onClick={() => handleCheck(gw.name)}
                >
                  检测连接
                </Button>
                <Button
                  variant="contained"
                  color="primary"
                  disabled={
                    !gw.enabled ||
                    (gw.libraries || []).length === 0 ||
                    scanning === gw.name ||
                    scanStatus.scanning
                  }
                  startIcon={
                    scanning === gw.name ? <CircularProgress size={16} /> : <SyncIcon />
                  }
                  onClick={() => handleScan(gw)}
                >
                  扫描云库
                </Button>
              </Box>

              {(gw.libraries || []).length > 0 && (
                <TableContainer component={Paper} elevation={0} style={{ marginTop: 12 }}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>云库</TableCell>
                        <TableCell>类型</TableCell>
                        <TableCell align="right">文件数</TableCell>
                        <TableCell align="right">歌曲数</TableCell>
                        <TableCell align="right">最后扫描</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {gw.libraries.map((lib) => (
                        <TableRow key={lib.id}>
                          <TableCell>{lib.name}</TableCell>
                          <TableCell>
                            <Chip
                              size="small"
                              label={lib.mediaType === 'audiobook' ? '有声书' : '音乐'}
                              variant="outlined"
                            />
                          </TableCell>
                          <TableCell align="right">{lib.totalFiles}</TableCell>
                          <TableCell align="right">{lib.totalSongs}</TableCell>
                          <TableCell align="right">{fmtTime(lib.lastScanAt)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>
        ))
      )}
    </Box>
  )
}

export default CloudSourcePage
