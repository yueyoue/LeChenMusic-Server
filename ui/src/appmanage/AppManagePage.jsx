import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button,
  TextField, Switch, FormControlLabel, Divider, IconButton,
  Chip, LinearProgress
} from '@material-ui/core'
import CloudUploadIcon from '@material-ui/icons/CloudUpload'
import DeleteIcon from '@material-ui/icons/Delete'
import AddIcon from '@material-ui/icons/Add'
import PhoneAndroidIcon from '@material-ui/icons/PhoneAndroid'

const useStyles = makeStyles((theme) => ({
  root: { padding: 24, maxWidth: 900, margin: '0 auto' },
  section: { marginBottom: 32 },
  sectionTitle: {
    fontSize: 18, fontWeight: 700, marginBottom: 16,
    display: 'flex', alignItems: 'center', gap: 8
  },
  card: { marginBottom: 16 },
  uploadArea: {
    border: '2px dashed rgba(128,128,128,0.3)',
    borderRadius: 12, padding: 32, textAlign: 'center',
    cursor: 'pointer', transition: 'all 0.2s',
    '&:hover': { borderColor: theme.palette.primary.main, background: 'rgba(25,118,210,0.04)' }
  },
  slideCard: {
    display: 'flex', alignItems: 'center', gap: 12,
    padding: '8px 12px', marginBottom: 8,
    background: theme.palette.background.default, borderRadius: 8
  },
  slideImage: { width: 80, height: 45, borderRadius: 4, objectFit: 'cover', background: '#333' },
  preview: { maxWidth: 300, maxHeight: 200, borderRadius: 8, marginTop: 8 }
}))

const AppManagePage = () => {
  const classes = useStyles()
  const [config, setConfig] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState('')
  const [toast, setToast] = useState('')

  // Form states
  const [versionName, setVersionName] = useState('')
  const [versionCode, setVersionCode] = useState('')
  const [updateLog, setUpdateLog] = useState('')
  const [forceUpdate, setForceUpdate] = useState(false)
  const [splashDuration, setSplashDuration] = useState(3)
  const [slideTitle, setSlideTitle] = useState('')
  const [slideLink, setSlideLink] = useState('')

  useEffect(() => { fetchConfig() }, [])

  const fetchConfig = async () => {
    try {
      const token = localStorage.getItem('token')
      const res = await fetch('/api/app/config', {
        headers: { 'X-ND-Authorization': `Bearer ${token}` }
      })
      if (res.ok) {
        const data = await res.json()
        const c = data.data || {}
        setConfig(c)
        setVersionName(c.versionName || '')
        setVersionCode(String(c.versionCode || ''))
        setUpdateLog(c.updateLog || '')
        setForceUpdate(c.forceUpdate || false)
        setSplashDuration(c.splashDuration || 3)
      }
    } catch (err) {
      console.error('Failed to load config:', err)
    } finally {
      setLoading(false)
    }
  }

  const showToast = (msg) => {
    setToast(msg)
    setTimeout(() => setToast(''), 3000)
  }

  // Upload APK
  const handleApkUpload = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    if (!file.name.endsWith('.apk')) {
      showToast('请选择 .apk 文件')
      return
    }

    setUploading(true)
    setUploadProgress('正在上传...')
    try {
      const token = localStorage.getItem('token')
      const formData = new FormData()
      formData.append('apk', file)
      formData.append('versionName', versionName)
      formData.append('versionCode', versionCode)
      formData.append('updateLog', updateLog)
      formData.append('forceUpdate', String(forceUpdate))

      const res = await fetch('/api/app/apk', {
        method: 'POST',
        headers: { 'X-ND-Authorization': `Bearer ${token}` },
        body: formData
      })
      if (res.ok) {
        showToast('APK 上传成功！')
        fetchConfig()
      } else {
        showToast('上传失败')
      }
    } catch (err) {
      showToast('上传失败: ' + err.message)
    } finally {
      setUploading(false)
      setUploadProgress('')
    }
  }

  // Upload splash image
  const handleSplashUpload = async (e) => {
    const file = e.target.files[0]
    if (!file) return

    setUploading(true)
    try {
      const token = localStorage.getItem('token')
      const formData = new FormData()
      formData.append('image', file)
      formData.append('duration', String(splashDuration))

      const res = await fetch('/api/app/splash', {
        method: 'POST',
        headers: { 'X-ND-Authorization': `Bearer ${token}` },
        body: formData
      })
      if (res.ok) {
        showToast('开屏图片上传成功！')
        fetchConfig()
      }
    } catch (err) {
      showToast('上传失败')
    } finally {
      setUploading(false)
    }
  }

  // Add slide
  const handleAddSlide = async (e) => {
    const file = e.target.files[0]
    if (!file) return

    setUploading(true)
    try {
      const token = localStorage.getItem('token')
      const formData = new FormData()
      formData.append('image', file)
      formData.append('title', slideTitle)
      formData.append('link', slideLink)

      const res = await fetch('/api/app/slide', {
        method: 'POST',
        headers: { 'X-ND-Authorization': `Bearer ${token}` },
        body: formData
      })
      if (res.ok) {
        showToast('幻灯片添加成功！')
        setSlideTitle('')
        setSlideLink('')
        fetchConfig()
      }
    } catch (err) {
      showToast('添加失败')
    } finally {
      setUploading(false)
    }
  }

  // Delete slide
  const handleDeleteSlide = async (slideId) => {
    try {
      const token = localStorage.getItem('token')
      await fetch(`/api/app/slide/${slideId}`, {
        method: 'DELETE',
        headers: { 'X-ND-Authorization': `Bearer ${token}` }
      })
      showToast('幻灯片已删除')
      fetchConfig()
    } catch (err) {
      showToast('删除失败')
    }
  }

  // Save config (version info, splash duration)
  const handleSaveConfig = async () => {
    setSaving(true)
    try {
      const token = localStorage.getItem('token')
      const newConfig = {
        ...config,
        versionName,
        versionCode: parseInt(versionCode) || 0,
        updateLog,
        forceUpdate,
        splashDuration
      }
      const res = await fetch('/api/app/config', {
        method: 'PUT',
        headers: {
          'X-ND-Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(newConfig)
      })
      if (res.ok) {
        showToast('配置已保存')
        fetchConfig()
      }
    } catch (err) {
      showToast('保存失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <Box p={4} textAlign="center"><Typography>加载中...</Typography></Box>

  return (
    <Box className={classes.root}>
      <Typography variant="h5" style={{ fontWeight: 700, marginBottom: 24, display: 'flex', alignItems: 'center', gap: 8 }}>
        <PhoneAndroidIcon /> APP 管理
      </Typography>

      {toast && (
        <Box mb={2} p={1.5} style={{ background: '#4caf50', borderRadius: 8, color: '#fff', textAlign: 'center' }}>
          {toast}
        </Box>
      )}

      {/* Version & APK */}
      <Box className={classes.section}>
        <Typography className={classes.sectionTitle}>📦 版本 & APK</Typography>
        <Card className={classes.card}>
          <CardContent>
            <Box display="flex" gap={2} mb={2}>
              <TextField label="版本名" value={versionName} onChange={e => setVersionName(e.target.value)}
                size="small" variant="outlined" style={{ width: 150 }} />
              <TextField label="版本号" value={versionCode} onChange={e => setVersionCode(e.target.value)}
                size="small" variant="outlined" style={{ width: 120 }} type="number" />
              <FormControlLabel
                control={<Switch checked={forceUpdate} onChange={e => setForceUpdate(e.target.checked)} color="primary" />}
                label="强制更新"
              />
            </Box>
            <TextField label="更新日志" value={updateLog} onChange={e => setUpdateLog(e.target.value)}
              multiline rows={3} fullWidth variant="outlined" size="small" style={{ marginBottom: 16 }} />

            {config?.apkFileName && (
              <Box mb={2}>
                <Chip label={`当前: ${config.apkFileName} (${(config.apkFileSize / 1024 / 1024).toFixed(1)} MB)`}
                  color="primary" variant="outlined" />
                <Typography variant="caption" display="block" style={{ marginTop: 4, color: 'text.secondary' }}>
                  上传时间: {config.apkUploadTime}
                </Typography>
              </Box>
            )}

            <Box display="flex" gap={2}>
              <Button variant="contained" color="primary" onClick={handleSaveConfig} disabled={saving}>
                {saving ? '保存中...' : '保存配置'}
              </Button>
              <label>
                <input type="file" accept=".apk" hidden onChange={handleApkUpload} />
                <Button variant="outlined" component="span" startIcon={<CloudUploadIcon />} disabled={uploading}>
                  {uploading ? '上传中...' : '上传 APK'}
                </Button>
              </label>
            </Box>
            {uploadProgress && <LinearProgress style={{ marginTop: 8 }} />}
            {uploadProgress && <Typography variant="caption">{uploadProgress}</Typography>}
          </CardContent>
        </Card>
      </Box>

      <Divider style={{ marginBottom: 32 }} />

      {/* Splash Screen */}
      <Box className={classes.section}>
        <Typography className={classes.sectionTitle}>🖼️ 开屏页</Typography>
        <Card className={classes.card}>
          <CardContent>
            <Box display="flex" gap={2} alignItems="center" mb={2}>
              <TextField label="展示时长(秒)" value={splashDuration}
                onChange={e => setSplashDuration(parseInt(e.target.value) || 3)}
                size="small" variant="outlined" type="number" style={{ width: 150 }} />
              <label>
                <input type="file" accept="image/*" hidden onChange={handleSplashUpload} />
                <Button variant="outlined" component="span" startIcon={<CloudUploadIcon />}>
                  上传开屏图
                </Button>
              </label>
            </Box>
            {config?.splashImageUrl && (
              <Box>
                <Typography variant="caption" color="textSecondary">当前开屏图：</Typography>
                <img src={config.splashImageUrl} alt="splash"
                  className={classes.preview} style={{ display: 'block' }} />
              </Box>
            )}
          </CardContent>
        </Card>
      </Box>

      <Divider style={{ marginBottom: 32 }} />

      {/* Carousel Slides */}
      <Box className={classes.section}>
        <Typography className={classes.sectionTitle}>🎠 幻灯片 (首页轮播)</Typography>
        <Card className={classes.card}>
          <CardContent>
            {config?.slides?.map((slide) => (
              <Box key={slide.id} className={classes.slideCard}>
                <img src={slide.imageUrl} alt={slide.title} className={classes.slideImage} />
                <Box flex={1}>
                  <Typography style={{ fontWeight: 600, fontSize: 14 }}>{slide.title || '无标题'}</Typography>
                  {slide.link && <Typography variant="caption" color="textSecondary">{slide.link}</Typography>}
                </Box>
                <IconButton size="small" onClick={() => handleDeleteSlide(slide.id)}>
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Box>
            ))}
            {(!config?.slides || config.slides.length === 0) && (
              <Typography color="textSecondary" style={{ textAlign: 'center', padding: 16 }}>
                暂无幻灯片
              </Typography>
            )}

            <Divider style={{ margin: '16px 0' }} />
            <Typography style={{ fontWeight: 600, marginBottom: 8 }}>添加幻灯片</Typography>
            <Box display="flex" gap={2} mb={2}>
              <TextField label="标题" value={slideTitle} onChange={e => setSlideTitle(e.target.value)}
                size="small" variant="outlined" style={{ flex: 1 }} />
              <TextField label="链接(可选)" value={slideLink} onChange={e => setSlideLink(e.target.value)}
                size="small" variant="outlined" style={{ flex: 1 }} />
            </Box>
            <label>
              <input type="file" accept="image/*" hidden onChange={handleAddSlide} />
              <Button variant="outlined" component="span" startIcon={<AddIcon />} size="small">
                选择图片并添加
              </Button>
            </label>
          </CardContent>
        </Card>
      </Box>
    </Box>
  )
}

export default AppManagePage
