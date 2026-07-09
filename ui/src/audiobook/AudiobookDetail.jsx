import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, IconButton,
  Chip, Button, LinearProgress, Tooltip, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PauseIcon from '@material-ui/icons/Pause'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import BookmarkIcon from '@material-ui/icons/Bookmark'
import ShareIcon from '@material-ui/icons/Share'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import AccessTimeIcon from '@material-ui/icons/AccessTime'
import PersonIcon from '@material-ui/icons/Person'
import EditIcon from '@material-ui/icons/Edit'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16 },
  header: { display: 'flex', gap: 20, marginBottom: 24 },
  cover: {
    width: 160, height: 210, borderRadius: 8, objectFit: 'cover',
    backgroundColor: theme.palette.grey[300], flexShrink: 0,
    boxShadow: '0 4px 20px rgba(0,0,0,0.15)',
  },
  coverPlaceholder: {
    width: 160, height: 210, borderRadius: 8, flexShrink: 0,
    backgroundColor: theme.palette.grey[200],
    display: 'flex', alignItems: 'center', justifyContent: 'center',
  },
  meta: { flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center' },
  title: { fontSize: 22, fontWeight: 700, marginBottom: 8 },
  author: { fontSize: 14, color: theme.palette.text.secondary, marginBottom: 4 },
  narrator: { fontSize: 13, color: theme.palette.text.secondary, marginBottom: 8 },
  genre: { marginBottom: 8 },
  stats: { display: 'flex', gap: 16, fontSize: 13, color: theme.palette.text.secondary },
  actions: { display: 'flex', gap: 8, marginBottom: 24 },
  playBtn: { borderRadius: 20, textTransform: 'none', fontWeight: 600 },
  chapterList: { marginTop: 8 },
  chapterItem: {
    display: 'flex', alignItems: 'center', padding: '10px 12px',
    borderRadius: 8, cursor: 'pointer', marginBottom: 4,
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  chapterActive: {
    backgroundColor: theme.palette.action.selected,
    borderLeft: `3px solid ${theme.palette.primary.main}`,
  },
  chapterNum: { width: 36, fontSize: 13, color: theme.palette.text.secondary, textAlign: 'center' },
  chapterTitle: { flex: 1, fontSize: 14, marginLeft: 8 },
  chapterDuration: { fontSize: 12, color: theme.palette.text.secondary, marginLeft: 8 },
  chapterPlay: { marginLeft: 8 },
  progress: { marginBottom: 16 },
  backBtn: { marginBottom: 16 },
  sectionTitle: { fontSize: 16, fontWeight: 600, marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 },
  topBar: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 },
}))

const formatDuration = (seconds) => {
  if (!seconds) return ''
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

const AudiobookDetail = ({ id, onBack, onPlay, currentChapterId, isPlaying }) => {
  const classes = useStyles()
  const [book, setBook] = useState(null)
  const [chapters, setChapters] = useState([])
  const [progress, setProgress] = useState(null)
  const [loading, setLoading] = useState(true)
  const [bgColor, setBgColor] = useState(null)
  const [editOpen, setEditOpen] = useState(false)
  const [editForm, setEditForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [rescanning, setRescanning] = useState(false)

  const getToken = () => localStorage.getItem('token')

  const getAuthHeaders = () => ({
    'X-ND-Authorization': `Bearer ${getToken()}`
  })

  // Build cover URL with auth token as query param (for <img> tags which can't send headers)
  const getCoverUrl = (bookId) => {
    const token = getToken()
    return `/api/audiobook/${bookId}/cover${token ? '?token=' + token : ''}`
  }

  // Extract dominant color from cover
  const extractColor = (imageUrl) => {
    try {
      const img = new Image()
      img.crossOrigin = 'anonymous'
      img.onload = () => {
        const canvas = document.createElement('canvas')
        canvas.width = 1
        canvas.height = 1
        const ctx = canvas.getContext('2d')
        ctx.drawImage(img, 0, 0, 1, 1)
        const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
        setBgColor(`linear-gradient(135deg, rgb(${r},${g},${b}) 0%, rgba(${r},${g},${b},0.3) 100%)`)
      }
      img.src = imageUrl
    } catch (e) {
      // ignore
    }
  }

  useEffect(() => {
    const fetchData = async () => {
      try {
        const headers = getAuthHeaders()
        const [bookRes, progressRes] = await Promise.all([
          fetch(`/api/audiobook/${id}`, { headers }),
          fetch(`/api/audiobook/${id}/progress`, { headers }),
        ])
        if (bookRes.ok) {
          const data = await bookRes.json()
          if (data.data) {
            setBook(data.data.book)
            setChapters(data.data.chapters || [])
            // Always try to extract color from cover
            extractColor(getCoverUrl(id))
          }
        }
        if (progressRes.ok) {
          const pData = await progressRes.json()
          setProgress(pData.data)
        }
      } catch (err) {
        console.error('Failed to load audiobook:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchData()
  }, [id])

  const handlePlay = (chapter) => {
    if (onPlay) {
      onPlay(book, chapters, chapter)
    }
  }

  const handleContinue = () => {
    if (progress && progress.chapterId) {
      const chapter = chapters.find(c => c.id === progress.chapterId)
      if (chapter) {
        handlePlay(chapter)
        return
      }
    }
    if (chapters.length > 0) handlePlay(chapters[0])
  }

  const handleEditOpen = () => {
    setEditForm({
      title: book.title || '',
      author: book.author || '',
      narrator: book.narrator || '',
      description: book.description || '',
      genre: book.genre || '',
      series: book.series || '',
    })
    setEditOpen(true)
  }

  const handleEditSave = async () => {
    setSaving(true)
    try {
      const res = await fetch(`/api/audiobook/${id}/metadata`, {
        method: 'PUT',
        headers: {
          ...getAuthHeaders(),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(editForm),
      })
      if (res.ok) {
        const data = await res.json()
        if (data.data) {
          setBook(data.data)
        }
        setEditOpen(false)
      }
    } catch (err) {
      console.error('Failed to save:', err)
    } finally {
      setSaving(false)
    }
  }

  const handleRescan = async () => {
    setRescanning(true)
    try {
      const res = await fetch(`/api/audiobook/${id}/rescan`, {
        method: 'POST',
        headers: getAuthHeaders(),
      })
      if (res.ok) {
        const data = await res.json()
        if (data.data) {
          setBook(data.data.book)
          setChapters(data.data.chapters || [])
        }
      }
    } catch (err) {
      console.error('Failed to rescan:', err)
    } finally {
      setRescanning(false)
    }
  }

  if (loading) {
    return <Box p={4} textAlign="center"><Typography>Loading...</Typography></Box>
  }

  if (!book) {
    return <Box p={4} textAlign="center"><Typography>Audiobook not found</Typography></Box>
  }

  return (
    <Box className={classes.root} style={bgColor ? { background: bgColor, borderRadius: 12, padding: 16 } : {}}>
      <Box className={classes.topBar}>
        <IconButton className={classes.backBtn} onClick={onBack || (() => { window.location.hash = '#/audiobook' })}>
          <ArrowBackIcon />
        </IconButton>
        <Box>
          <Tooltip title="重新扫描章节">
            <IconButton onClick={handleRescan} disabled={rescanning}>
              <span style={{ fontSize: 18 }}>{rescanning ? '⏳' : '🔄'}</span>
            </IconButton>
          </Tooltip>
          <Tooltip title="编辑有声书信息">
            <IconButton onClick={handleEditOpen}>
              <EditIcon />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      <Box className={classes.header}>
        <img
          src={getCoverUrl(book.id)}
          alt={book.title}
          className={classes.cover}
          onError={(e) => {
            e.target.style.display = 'none'
            e.target.nextSibling.style.display = 'flex'
          }}
        />
        <Box className={classes.coverPlaceholder} style={{ display: 'none' }}>
          <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
        </Box>
        <Box className={classes.meta}>
          <Typography className={classes.title}>{book.title}</Typography>
          {book.author && (
            <Typography className={classes.author}>
              <PersonIcon style={{ fontSize: 14, verticalAlign: 'middle', marginRight: 4 }} />
              {book.author}
            </Typography>
          )}
          {book.narrator && (
            <Typography className={classes.narrator}>
              🎙️ {book.narrator}
            </Typography>
          )}
          {book.genre && <Chip label={book.genre} size="small" className={classes.genre} />}
          <Box className={classes.stats}>
            <span>📑 {book.chapterCount} 章</span>
            {book.totalDuration > 0 && (
              <span><AccessTimeIcon style={{ fontSize: 14, verticalAlign: 'middle' }} /> {formatDuration(book.totalDuration)}</span>
            )}
          </Box>
        </Box>
      </Box>

      {book.description && (
        <Typography style={{ fontSize: 13, color: 'text.secondary', marginBottom: 16, lineHeight: 1.6 }}>
          {book.description}
        </Typography>
      )}

      {progress && (
        <Box className={classes.progress}>
          <Typography style={{ fontSize: 12, color: 'text.secondary', marginBottom: 4 }}>
            上次听到 第{progress.chapterNumber}章 · {formatDuration(progress.position)}
          </Typography>
          <LinearProgress
            variant="determinate"
            value={chapters.length > 0 ? (progress.chapterNumber / chapters.length) * 100 : 0}
            style={{ height: 4, borderRadius: 2 }}
          />
        </Box>
      )}

      <Box className={classes.actions}>
        <Button
          variant="contained"
          color="primary"
          className={classes.playBtn}
          startIcon={<PlayArrowIcon />}
          onClick={handleContinue}
        >
          {progress ? '继续播放' : '从头播放'}
        </Button>
        <Button
          variant="outlined"
          className={classes.playBtn}
          startIcon={<QueueMusicIcon />}
          onClick={() => chapters.length > 0 && handlePlay(chapters[0])}
        >
          播放全部
        </Button>
      </Box>

      <Typography className={classes.sectionTitle}>
        📑 章节列表 ({chapters.length})
      </Typography>

      <Box className={classes.chapterList}>
        {chapters.map((chapter, idx) => {
          const isActive = currentChapterId === chapter.id
          return (
            <Box
              key={chapter.id}
              className={`${classes.chapterItem} ${isActive ? classes.chapterActive : ''}`}
              onClick={() => handlePlay(chapter)}
            >
              <Typography className={classes.chapterNum}>{chapter.chapterNumber || idx + 1}</Typography>
              <Typography className={classes.chapterTitle}>{chapter.title}</Typography>
              {chapter.duration > 0 && (
                <Typography className={classes.chapterDuration}>{formatDuration(chapter.duration)}</Typography>
              )}
              <Box className={classes.chapterPlay}>
                {isActive && isPlaying ? (
                  <PauseIcon style={{ fontSize: 20, color: 'primary.main' }} />
                ) : (
                  <PlayArrowIcon style={{ fontSize: 20, color: 'action.active' }} />
                )}
              </Box>
            </Box>
          )
        })}
      </Box>

      {/* Edit Dialog */}
      <Dialog open={editOpen} onClose={() => setEditOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>编辑有声书信息</DialogTitle>
        <DialogContent>
          <TextField
            label="有声书名称"
            value={editForm.title || ''}
            onChange={(e) => setEditForm({ ...editForm, title: e.target.value })}
            fullWidth
            margin="normal"
          />
          <TextField
            label="作者"
            value={editForm.author || ''}
            onChange={(e) => setEditForm({ ...editForm, author: e.target.value })}
            fullWidth
            margin="normal"
          />
          <TextField
            label="演播者"
            value={editForm.narrator || ''}
            onChange={(e) => setEditForm({ ...editForm, narrator: e.target.value })}
            fullWidth
            margin="normal"
          />
          <TextField
            label="分类"
            value={editForm.genre || ''}
            onChange={(e) => setEditForm({ ...editForm, genre: e.target.value })}
            fullWidth
            margin="normal"
            select
            SelectProps={{ native: true }}
          >
            <option value="">请选择</option>
            <option value="有声读物">有声读物</option>
            <option value="评书">评书</option>
            <option value="相声">相声</option>
            <option value="戏曲">戏曲</option>
            <option value="儿童">儿童</option>
            <option value="教育">教育</option>
          </TextField>
          <TextField
            label="系列"
            value={editForm.series || ''}
            onChange={(e) => setEditForm({ ...editForm, series: e.target.value })}
            fullWidth
            margin="normal"
          />
          <TextField
            label="简介"
            value={editForm.description || ''}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
            fullWidth
            margin="normal"
            multiline
            rows={3}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditOpen(false)}>取消</Button>
          <Button onClick={handleEditSave} color="primary" disabled={saving}>
            {saving ? '保存中...' : '保存'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default AudiobookDetail
