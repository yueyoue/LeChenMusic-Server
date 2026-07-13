import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import {
  Typography, Box, Card, CardContent, makeStyles, IconButton,
  Chip, Button, LinearProgress, Tooltip, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField, List, ListItem, ListItemText
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PauseIcon from '@material-ui/icons/Pause'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import AccessTimeIcon from '@material-ui/icons/AccessTime'
import PersonIcon from '@material-ui/icons/Person'
import EditIcon from '@material-ui/icons/Edit'
import RefreshIcon from '@material-ui/icons/Refresh'
import FavoriteIcon from '@material-ui/icons/Favorite'
import FavoriteBorderIcon from '@material-ui/icons/FavoriteBorder'
import subsonic from '../subsonic'
import { playTracks, addTracks } from '../actions'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16 },
  header: { display: 'flex', gap: 20, marginBottom: 24 },
  cover: {
    width: 160, height: 160, borderRadius: 8, objectFit: 'cover',
    backgroundColor: theme.palette.grey[300], flexShrink: 0,
    boxShadow: '0 4px 20px rgba(0,0,0,0.15)',
  },
  coverPlaceholder: {
    width: 160, height: 160, borderRadius: 8, flexShrink: 0,
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
  progress: { marginBottom: 16 },
  topBar: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 16 },
}))

const formatDuration = (seconds) => {
  if (!seconds) return ''
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

/**
 * Convert audiobook chapters to song-like format for the music player
 */
const chaptersToSongs = (book, chapters) => {
  const songs = {}
  const ids = []
  const token = localStorage.getItem('token')
  const coverUrl = `/api/audiobook/${book.id}/cover${token ? '?token=' + token : ''}`
  
  chapters.forEach((ch) => {
    const songId = ch.id
    ids.push(songId)
    songs[songId] = {
      id: songId,
      title: ch.title,
      artist: book.narrator || book.author || '未知',
      album: book.title,
      albumId: `audiobook-${book.id}`,
      artistId: '',
      track: ch.chapterNumber,
      duration: ch.duration || 0,
      year: book.year || 0,
      genre: book.genre || '有声读物',
      contentType: 'audio/mpeg',
      suffix: ch.format || 'mp3',
      size: ch.fileSize || 0,
      bitRate: 0,
      // Mark as audiobook for the player
      isAudiobook: true,
      audiobookId: book.id,
      chapterId: ch.id,
      // Use audiobook cover URL directly
      coverArtUrl: coverUrl,
    }
  })
  return { songs, ids }
}

const AudiobookDetail = ({ id, onBack }) => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const [book, setBook] = useState(null)
  const [chapters, setChapters] = useState([])
  const [progress, setProgress] = useState(null)
  const [loading, setLoading] = useState(true)
  const [bgColor, setBgColor] = useState(null)
  const [editOpen, setEditOpen] = useState(false)
  const [editForm, setEditForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [rescanning, setRescanning] = useState(false)
  const [isStarred, setIsStarred] = useState(false)

  const currentPlaying = useSelector((state) => state.player.currentPlaying)
  const isPlayingCurrent = currentPlaying?.albumId === `audiobook-${id}`

  const getToken = () => localStorage.getItem('token')
  const getAuthHeaders = () => ({ 'X-ND-Authorization': `Bearer ${getToken()}` })

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
        canvas.width = 1; canvas.height = 1
        const ctx = canvas.getContext('2d')
        ctx.drawImage(img, 0, 0, 1, 1)
        const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
        setBgColor(`linear-gradient(135deg, rgb(${r},${g},${b}) 0%, rgba(${r},${g},${b},0.3) 100%)`)
      }
      img.src = imageUrl
    } catch (e) {}
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
            setIsStarred(!!data.data.book?.starred)
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

  // Play a specific chapter using the music player
  const handlePlayChapter = useCallback((chapter) => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(playTracks(songs, ids, chapter.id))
  }, [book, chapters, dispatch])

  // Play all chapters from the beginning
  const handlePlayAll = useCallback(() => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(playTracks(songs, ids, ids[0]))
  }, [book, chapters, dispatch])

  // Continue from last position
  const handleContinue = useCallback(() => {
    if (progress && progress.chapterId) {
      const chapter = chapters.find(c => c.id === progress.chapterId)
      if (chapter) {
        // Pass saved position (in seconds) so the player can seek to it
        const { songs, ids } = chaptersToSongs(book, chapters)
        dispatch(playTracks(songs, ids, chapter.id, progress.position || 0))
        return
      }
    }
    handlePlayAll()
  }, [progress, chapters, book, dispatch, handlePlayAll])

  // Add to queue
  const handleAddToQueue = useCallback(() => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(addTracks(songs, ids))
  }, [book, chapters, dispatch])

  // Rescan chapters
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

  // Toggle favorite
  const handleToggleFavorite = async () => {
    try {
      const method = isStarred ? 'DELETE' : 'POST'
      const res = await fetch(`/api/audiobook/${id}/star`, { method, headers: getAuthHeaders() })
      if (res.ok) {
        // Refetch to get correct starred status from server
        const bookRes = await fetch(`/api/audiobook/${id}`, { headers: getAuthHeaders() })
        if (bookRes.ok) {
          const data = await bookRes.json()
          setIsStarred(!!data.data?.book?.starred)
        } else {
          setIsStarred(!isStarred)
        }
      }
    } catch (err) {
      console.error('Failed to toggle favorite:', err)
    }
  }

  // Edit metadata
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
        headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify(editForm),
      })
      if (res.ok) {
        const data = await res.json()
        if (data.data) setBook(data.data)
        setEditOpen(false)
      }
    } catch (err) {
      console.error('Failed to save:', err)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <Box p={4} textAlign="center"><Typography>Loading...</Typography></Box>
  }
  if (!book) {
    return <Box p={4} textAlign="center"><Typography>Audiobook not found</Typography></Box>
  }

  // Check which chapter is currently playing
  const currentPlayingId = currentPlaying?.id

  return (
    <Box className={classes.root} style={bgColor ? { background: bgColor, borderRadius: 12, padding: 16 } : {}}>
      <Box className={classes.topBar}>
        <IconButton onClick={onBack || (() => { window.location.hash = '#/audiobook' })}>
          <ArrowBackIcon />
        </IconButton>
        <Box>
          <Tooltip title="重新扫描章节">
            <IconButton onClick={handleRescan} disabled={rescanning}>
              <RefreshIcon style={rescanning ? { animation: 'spin 1s linear infinite' } : {}} />
            </IconButton>
          </Tooltip>
          <Tooltip title={isStarred ? '取消收藏' : '收藏'}>
            <IconButton onClick={handleToggleFavorite}>
              {isStarred ? (
                <FavoriteIcon style={{ color: '#ff4757' }} />
              ) : (
                <FavoriteBorderIcon />
              )}
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
        <img src={getCoverUrl(book.id)} alt={book.title} className={classes.cover}
          onError={(e) => { e.target.style.display = 'none'; e.target.nextSibling.style.display = 'flex' }} />
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
            <Typography className={classes.narrator}>🎙️ {book.narrator}</Typography>
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
        <Button variant="contained" color="primary" className={classes.playBtn}
          startIcon={<PlayArrowIcon />} onClick={handleContinue}>
          {progress ? '继续播放' : '从头播放'}
        </Button>
        <Button variant="outlined" className={classes.playBtn}
          startIcon={<QueueMusicIcon />} onClick={handleAddToQueue}>
          加入队列
        </Button>
      </Box>

      <Typography style={{ fontSize: 16, fontWeight: 600, marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
        📑 章节列表 ({chapters.length})
      </Typography>

      <Box className={classes.chapterList}>
        {chapters.map((chapter, idx) => {
          const isActive = currentPlayingId === chapter.id
          return (
            <Box key={chapter.id}
              className={`${classes.chapterItem} ${isActive ? classes.chapterActive : ''}`}
              onClick={() => handlePlayChapter(chapter)}>
              <Typography className={classes.chapterNum}>{chapter.chapterNumber || idx + 1}</Typography>
              <Typography className={classes.chapterTitle}>{chapter.title}</Typography>
              {chapter.duration > 0 && (
                <Typography className={classes.chapterDuration}>{formatDuration(chapter.duration)}</Typography>
              )}
              <Box style={{ marginLeft: 8 }}>
                {isActive ? (
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
          <TextField label="有声书名称" value={editForm.title || ''}
            onChange={(e) => setEditForm({ ...editForm, title: e.target.value })} fullWidth margin="normal" />
          <TextField label="作者" value={editForm.author || ''}
            onChange={(e) => setEditForm({ ...editForm, author: e.target.value })} fullWidth margin="normal" />
          <TextField label="演播者" value={editForm.narrator || ''}
            onChange={(e) => setEditForm({ ...editForm, narrator: e.target.value })} fullWidth margin="normal" />
          <TextField label="分类" value={editForm.genre || ''}
            onChange={(e) => setEditForm({ ...editForm, genre: e.target.value })} fullWidth margin="normal"
            select SelectProps={{ native: true }}>
            <option value="">请选择</option>
            <option value="有声读物">有声读物</option>
            <option value="评书">评书</option>
            <option value="相声">相声</option>
            <option value="戏曲">戏曲</option>
            <option value="儿童">儿童</option>
            <option value="教育">教育</option>
          </TextField>
          <TextField label="系列" value={editForm.series || ''}
            onChange={(e) => setEditForm({ ...editForm, series: e.target.value })} fullWidth margin="normal" />
          <TextField label="简介" value={editForm.description || ''}
            onChange={(e) => setEditForm({ ...editForm, description: e.target.value })} fullWidth margin="normal" multiline rows={3} />
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
