import React, { useState, useEffect, useCallback } from 'react'
import { Title, useNotify } from 'react-admin'
import {
  Button,
  Card,
  CardContent,
  Typography,
  Box,
  Switch,
  FormControlLabel,
  TextField,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  IconButton,
  Chip,
  Alert,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@material-ui/core'
import SettingsIcon from '@material-ui/icons/Settings'
import SaveIcon from '@material-ui/icons/Save'
import UndoIcon from '@material-ui/icons/Undo'
import CloudUploadIcon from '@material-ui/icons/CloudUpload'

const BackupPage = () => {
  const notify = useNotify()
  const [backups, setBackups] = useState([])
  const [config, setConfig] = useState({
    enabled: false,
    backup_dir: '/data/backups',
    keep_count: 7,
    interval: 'daily',
  })
  const [exporting, setExporting] = useState(false)
  const [importDialogOpen, setImportDialogOpen] = useState(false)
  const [importFile, setImportFile] = useState(null)
  const [importing, setImporting] = useState(false)

  const getToken = () => localStorage.getItem('token')

  const loadBackups = useCallback(async () => {
    try {
      const res = await fetch('/api/backup/list', {
        headers: { 'X-ND-Authorization': 'Bearer ' + getToken() },
      })
      const data = await res.json()
      setBackups(data.data || [])
    } catch (err) {
      // ignore
    }
  }, [])

  const loadConfig = useCallback(async () => {
    try {
      const res = await fetch('/api/backup/config', {
        headers: { 'X-ND-Authorization': 'Bearer ' + getToken() },
      })
      const data = await res.json()
      if (data.data) setConfig(data.data)
    } catch (err) {
      // ignore
    }
  }, [])

  useEffect(() => {
    loadBackups()
    loadConfig()
  }, [loadBackups, loadConfig])

  const handleExport = async () => {
    setExporting(true)
    try {
      const res = await fetch('/api/backup/export', {
        method: 'POST',
        headers: { 'X-ND-Authorization': 'Bearer ' + getToken() },
      })
      const data = await res.json()
      if (data.data) {
        notify('备份成功: ' + data.data.file_path, 'success')
        loadBackups()
      }
    } catch (err) {
      notify('备份失败', 'error')
    }
    setExporting(false)
  }

  const handleSaveConfig = async () => {
    try {
      await fetch('/api/backup/config', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-ND-Authorization': 'Bearer ' + getToken(),
        },
        body: JSON.stringify(config),
      })
      notify('配置已保存', 'success')
    } catch (err) {
      notify('保存失败', 'error')
    }
  }

  const handleImport = async () => {
    if (!importFile) return
    setImporting(true)
    try {
      const res = await fetch('/api/backup/import', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-ND-Authorization': 'Bearer ' + getToken(),
        },
        body: JSON.stringify({
          file_path: importFile.path,
          import_users: true,
          overwrite_users: true,
        }),
      })
      const data = await res.json()
      if (data.data) {
        const r = data.data
        notify(
          '恢复成功! 用户:' + r.users_imported + ' 歌单:' + r.playlists_imported + ' 收藏:' + r.annotations_imported,
          'success'
        )
        setImportDialogOpen(false)
        setImportFile(null)
      }
    } catch (err) {
      notify('恢复失败', 'error')
    }
    setImporting(false)
  }

  const formatSize = (bytes) => {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }

  const formatDate = (dateStr) => {
    try {
      return new Date(dateStr).toLocaleString('zh-CN')
    } catch (e) {
      return dateStr
    }
  }

  return (
    <div>
      <Title title="备份与恢复" />
      <Box p={2}>
        <Card style={{ marginBottom: 16 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              备份操作
            </Typography>
            <Box display="flex" style={{ gap: 16 }} flexWrap="wrap">
              <Button
                variant="contained"
                color="primary"
                startIcon={exporting ? <CircularProgress size={20} /> : <SaveIcon />}
                onClick={handleExport}
                disabled={exporting}
              >
                {exporting ? '备份中...' : '立即备份'}
              </Button>
              <Button
                variant="outlined"
                startIcon={<UndoIcon />}
                onClick={() => setImportDialogOpen(true)}
              >
                恢复备份
              </Button>
            </Box>
            <Typography variant="body2" color="textSecondary" style={{ marginTop: 8 }}>
              备份内容：用户账号（含密码）、歌单、收藏、有声书进度和书签
            </Typography>
          </CardContent>
        </Card>

        <Card style={{ marginBottom: 16 }}>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              <SettingsIcon style={{ verticalAlign: 'middle', marginRight: 8 }} />
              定时备份配置
            </Typography>
            <Box display="flex" flexDirection="column" style={{ gap: 16, maxWidth: 400 }}>
              <FormControlLabel
                control={
                  <Switch
                    checked={config.enabled}
                    onChange={(e) => setConfig({ ...config, enabled: e.target.checked })}
                    color="primary"
                  />
                }
                label="启用定时备份"
              />
              <TextField
                label="备份目录"
                value={config.backup_dir}
                onChange={(e) => setConfig({ ...config, backup_dir: e.target.value })}
                size="small"
                fullWidth
                helperText="服务器上的备份存储路径"
              />
              <TextField
                label="保留份数"
                type="number"
                value={config.keep_count}
                onChange={(e) => setConfig({ ...config, keep_count: parseInt(e.target.value) || 7 })}
                size="small"
                style={{ width: 120 }}
                helperText="自动清理旧备份，0=不限"
              />
              <Button variant="outlined" onClick={handleSaveConfig} style={{ alignSelf: 'flex-start' }}>
                保存配置
              </Button>
            </Box>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              备份文件列表
            </Typography>
            {backups.length === 0 ? (
              <Alert severity="info">暂无备份文件，点击上方"立即备份"创建第一个备份</Alert>
            ) : (
              <TableContainer component={Paper} variant="outlined">
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>文件名</TableCell>
                      <TableCell>大小</TableCell>
                      <TableCell>创建时间</TableCell>
                      <TableCell align="right">操作</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {backups.map((bk) => (
                      <TableRow key={bk.name}>
                        <TableCell>
                          <Chip label={bk.name} size="small" variant="outlined" />
                        </TableCell>
                        <TableCell>{formatSize(bk.size)}</TableCell>
                        <TableCell>{formatDate(bk.created_at)}</TableCell>
                        <TableCell align="right">
                          <IconButton
                            size="small"
                            title="恢复此备份"
                            onClick={() => {
                              setImportFile(bk)
                              setImportDialogOpen(true)
                            }}
                          >
                            <CloudUploadIcon />
                          </IconButton>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            )}
          </CardContent>
        </Card>
      </Box>

      <Dialog open={importDialogOpen} onClose={() => setImportDialogOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>恢复备份</DialogTitle>
        <DialogContent>
          {importFile ? (
            <Alert severity="warning" style={{ marginBottom: 16 }}>
              <Typography variant="body2">
                <strong>即将恢复以下备份：</strong>
              </Typography>
              <Typography variant="body2">文件：{importFile.name}</Typography>
              <Typography variant="body2" style={{ marginTop: 8 }}>
                <strong>注意：</strong>恢复会导入备份中的用户、歌单和收藏数据。已存在的用户会被覆盖。
              </Typography>
            </Alert>
          ) : (
            <Typography>请从备份列表中选择一个备份文件</Typography>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setImportDialogOpen(false)}>取消</Button>
          <Button
            onClick={handleImport}
            color="primary"
            variant="contained"
            disabled={!importFile || importing}
            startIcon={importing ? <CircularProgress size={20} /> : <UndoIcon />}
          >
            {importing ? '恢复中...' : '确认恢复'}
          </Button>
        </DialogActions>
      </Dialog>
    </div>
  )
}

export default BackupPage
