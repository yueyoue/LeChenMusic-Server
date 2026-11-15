import React, { useCallback, useEffect, useState } from 'react'
import {
  Typography, Box, makeStyles, Button, TextField, Radio, RadioGroup,
  FormControlLabel, FormControl, FormLabel, Card, CardContent, LinearProgress, Chip,
} from '@material-ui/core'
import { Alert } from '@material-ui/lab'
import SaveIcon from '@material-ui/icons/Save'
import RefreshIcon from '@material-ui/icons/Refresh'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

// 「权限设置」区（仅管理员）：控制非管理员能否使用在线下载，以及每日限额。
// 配置保存在服务端 property 表，保存后立即生效（无需重启）。

const useStyles = makeStyles((theme) => ({
  card: { marginBottom: 16 },
  sectionTitle: { fontWeight: 700, marginBottom: 4, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 },
  hint: { fontSize: 12, color: theme.palette.text.secondary, marginTop: 6 },
  optionRow: { marginBottom: 4 },
  optionDesc: { fontSize: 12, color: theme.palette.text.secondary, margin: '0 0 8px 32px' },
  quotaRow: { display: 'flex', alignItems: 'center', gap: 16, marginTop: 8, marginLeft: 32, flexWrap: 'wrap' },
  statusRow: { display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', marginTop: 12 },
  actions: { display: 'flex', alignItems: 'center', gap: 12, marginTop: 20 },
}))

const MODES = [
  {
    value: 'admin-or-approved',
    label: '仅管理员可下载',
    desc: '普通用户只能搜索查看，点下载会被拒绝（默认，最安全）。',
  },
  {
    value: 'open',
    label: '所有登录用户可下载',
    desc: '任何登录用户都能把歌曲下载到自己的曲库。',
  },
  {
    value: 'quota',
    label: '所有登录用户可下载，但每日限额',
    desc: '每个用户每天最多下载指定首数，超出后当天不能再下载。',
  },
]

const OnlineSettingsSection = () => {
  const classes = useStyles()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [permission, setPermission] = useState('admin-or-approved')
  const [dailyQuota, setDailyQuota] = useState(0)
  const [saved, setSaved] = useState(null)
  const [error, setError] = useState('')
  const [toast, setToast] = useState('')

  const applyResponse = (json) => {
    setPermission(json.permission || 'admin-or-approved')
    setDailyQuota(json.dailyQuota || 0)
    setSaved(json)
  }

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/settings`)
      applyResponse(res.json || {})
    } catch (e) {
      setError('加载权限设置失败：' + (e.message || ''))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(''), 4000)
    return () => clearTimeout(timer)
  }, [toast])

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      const res = await httpClient(`${REST_URL}/online/settings`, {
        method: 'PUT',
        body: JSON.stringify({ permission, dailyQuota: Number(dailyQuota) || 0 }),
      })
      applyResponse(res.json || {})
      setToast('权限设置已保存，立即生效')
    } catch (e) {
      setError('保存失败：' + (e.message || ''))
    } finally {
      setSaving(false)
    }
  }

  const modeLabel = (value) => (MODES.find((m) => m.value === value) || MODES[0]).label

  return (
    <>
      <Typography className={classes.sectionTitle}>
        下载权限
        <Button size="small" startIcon={<RefreshIcon />} onClick={load} disabled={loading || saving}>刷新</Button>
      </Typography>

      {toast && <Alert severity="success" style={{ marginBottom: 12 }} onClose={() => setToast('')}>{toast}</Alert>}
      {error && <Alert severity="error" style={{ marginBottom: 12 }} onClose={() => setError('')}>{error}</Alert>}

      {loading ? <LinearProgress /> : (
        <Card className={classes.card} elevation={0}>
          <CardContent>
            {saved && !saved.enabled && (
              <Alert severity="warning" style={{ marginBottom: 12 }}>
                在线下载总开关当前是关闭状态（配置文件 <code>OnlineDownload.Enabled=false</code>），
                保存下面的权限也不会生效，需改配置文件并重启服务。
              </Alert>
            )}

            <FormControl component="fieldset">
              <FormLabel component="legend" style={{ fontSize: 13 }}>非管理员下载权限</FormLabel>
              <RadioGroup value={permission} onChange={(e) => setPermission(e.target.value)}>
                {MODES.map((m) => (
                  <Box key={m.value} className={classes.optionRow}>
                    <FormControlLabel value={m.value} control={<Radio color="primary" size="small" />} label={m.label} />
                    <Typography className={classes.optionDesc}>{m.desc}</Typography>
                  </Box>
                ))}
              </RadioGroup>
            </FormControl>

            {permission === 'quota' && (
              <Box className={classes.quotaRow}>
                <TextField
                  label="每人每日限额（首）"
                  type="number"
                  size="small"
                  variant="outlined"
                  value={dailyQuota}
                  onChange={(e) => setDailyQuota(e.target.value)}
                  inputProps={{ min: 0, max: 1000 }}
                  style={{ width: 180 }}
                />
                <Typography className={classes.hint} style={{ margin: 0 }}>
                  填 0 表示不限量（等同「所有登录用户可下载」）
                </Typography>
              </Box>
            )}

            <Box className={classes.statusRow}>
              <Typography style={{ fontSize: 13 }}>当前生效：</Typography>
              <Chip size="small" color="primary" label={modeLabel(saved?.permission || permission)} />
              {(saved?.permission === 'quota' || permission === 'quota') && (
                <Chip size="small" variant="outlined"
                  label={(saved?.dailyQuota || 0) > 0 ? `每日 ${saved?.dailyQuota} 首` : '每日不限量'} />
              )}
              <Chip size="small" variant="outlined"
                label={saved?.enabled ? '总开关：已启用' : '总开关：已关闭'} />
            </Box>

            <Typography className={classes.hint}>
              保存后立即生效，无需重启；配置写入服务端 property 表，覆盖配置文件里的 OnlineDownload.Permission 默认值。
              手机 APP 与网页端遵循同一权限。
            </Typography>

            <Box className={classes.actions}>
              <Button variant="contained" color="primary" startIcon={<SaveIcon />}
                onClick={handleSave} disabled={saving || loading}>
                {saving ? '保存中…' : '保存设置'}
              </Button>
            </Box>
          </CardContent>
        </Card>
      )}
    </>
  )
}

export default OnlineSettingsSection