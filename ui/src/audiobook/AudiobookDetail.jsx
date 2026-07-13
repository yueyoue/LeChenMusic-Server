import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch, useSelector } from 'react-redux'
import {
  Typography, Box, Card, CardContent, CardMedia, makeStyles, IconButton,
  Chip, Button, LinearProgress, Tooltip, Dialog, DialogTitle,
  DialogContent, DialogActions, TextField, Collapse, useMediaQuery,
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PauseIcon from '@material-ui/icons/Pause'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import AccessTimeIcon from '@material-ui/icons/AccessTime'
import PersonIcon from '@material-ui/icons/Person'
import EditIcon from '@material-ui/icons/Edit'
import FavoriteIcon from '@material-ui/icons/Favorite'
import FavoriteBorderIcon from '@material-ui/icons/FavoriteBorder'
import RefreshIcon from '@material-ui/icons/Refresh'
import ScrapeDialog from '../scraper/ScrapeDialog'
import { playTracks, addTracks } from '../actions'

const useStyles = makeStyles(
  (theme) => ({
    root: {
      [theme.breakpoints.down('xs')]: {
        padding: '0.7em',
        minWidth: '20em',
      },
      [theme.breakpoints.up('sm')]: {
        padding: '1em',
        minWidth: '32em',
      },
    },
    topBar: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      marginBottom: 8,
    },
    cardContents: {
      display: 'flex',
    },
    details: {
      display: 'flex',
      flexDirection: 'column',
    },
    content: {
      flex: '2 0 auto',
    },
    coverParent: {
      [theme.breakpoints.down('xs')]: {
        height: '8em',
        width: '8em',
        minWidth: '8em',
      },
      [theme.breakpoints.up('sm')]: {
        height: '10em',
        width: '10em',
        minWidth: '10em',
      },
      [theme.breakpoints.up('lg')]: {
        height: '15em',
        width: '15em',
        minWidth: '15em',
      },
      backgroundColor: 'transparent',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    },
    cover: {
      objectFit: 'contain',
      display: 'block',
      width: '100%',
      height: '100%',
      backgroundColor: 'transparent',
      borderRadius: 4,
    },
    coverPlaceholder: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: '100%',
      height: '100%',
      backgroundColor: theme.palette.grey[200],
      borderRadius: 4,
    },
    recordName: {
      fontWeight: 700,
    },
    recordArtist: {
      color: theme.palette.text.secondary,
      fontSize: 14,
      marginBottom: 2,
    },
    recordMeta: {
      color: theme.palette.text.secondary,
      fontSize: 13,
      marginTop: 4,
    },
    genreChip: {
      marginTop: theme.spacing(0.5),
    },
    loveButton: {
      top: theme.spacing(-0.2),
      left: theme.spacing(0.5),
    },
    actions: {
      display: 'flex',
      gap: 8,
      marginTop: 12,
      flexWrap: 'wrap',
    },
    playBtn: {
      borderRadius: 20,
      textTransform: 'none',
      fontWeight: 600,
    },
    progress: {
      marginTop: 12,
      marginBottom: 4,
    },
    notes: {
      marginTop: 12,
      wordBreak: 'break-word',
      cursor: 'pointer',
      lineHeight: 1.6,
    },
    chapterSection: {
      marginTop: 24,
    },
    chapterHeader: {
      fontSize: 16,
      fontWeight: 600,
      marginBottom: 12,
      display: 'flex',
      alignItems: 'center',
      gap: 8,
    },
    chapterItem: {
      display: 'flex',
      alignItems: 'center',
      padding: '8px 12px',
      borderRadius: 8,
      cursor: 'pointer',
      marginBottom: 2,
      transition: 'background 0.15s',
      '&:hover': {
        backgroundColor: theme.palette.action.hover,
      },
    },
    chapterActive: {
      backgroundColor: theme.palette.action.selected,
      borderLeft: `3px solid ${theme.palette.primary.main}`,
    },
    chapterNum: {
      width: 36,
      fontSize: 13,
      color: theme.palette.text.secondary,
      textAlign: 'center',
      flexShrink: 0,
    },
    chapterTitle: {
      flex: 1,
      fontSize: 14,
      marginLeft: 8,
      overflow: 'hidden',
      whiteSpace: 'nowrap',
      textOverflow: 'ellipsis',
    },
    chapterDuration: {
      fontSize: 12,
      color: theme.palette.text.secondary,
      marginLeft: 8,
      flexShrink: 0,
    },
  }),
  {
    name: 'NDAudiobookDetail',
  }
)

const formatDuration = (seconds) => {
  if (!seconds) return ''
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

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
      isAudiobook: true,
      audiobookId: book.id,
      chapterId: ch.id,
      coverArtUrl: coverUrl,
    }
  })
  return { songs, ids }
}

const AudiobookDetail = ({ id, onBack }) => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('lg'))
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('xs'))

  const [book, setBook] = useState(null)
  const [chapters, setChapters] = useState([])
  const [progress, setProgress] = useState(null)
  const [loading, setLoading] = useState(true)
  const [editOpen, setEditOpen] = useState(false)
  const [editForm, setEditForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [rescanning, setRescanning] = useState(false)
  const [isStarred, setIsStarred] = useState(false)
  const [coverEditOpen, setCoverEditOpen] = useState(false)
  const [coverUrl, setCoverUrl] = useState('')
  const [coverUploading, setCoverUploading] = useState(false)
  const [scrapeOpen, setScrapeOpen] = useState(false)
  const [expandedNotes, setExpandedNotes] = useState(false)

  const currentPlaying = useSelector((state) => state.player.currentPlaying)
  const isPlayingCurrent = currentPlaying?.albumId === `audiobook-${id}`

  const getToken = () => localStorage.getItem('token')
  const getAuthHeaders = () => ({ 'X-ND-Authorization': `Bearer ${getToken()}` })
  const getCoverUrl = (bookId) => {
    const token = getToken()
    return `/api/audiobook/${bookId}/cover${token ? '?token=' + token : ''}`
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

  const handlePlayChapter = useCallback((chapter) => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(playTracks(songs, ids, chapter.id))
  }, [book, chapters, dispatch])

  const handlePlayAll = useCallback(() => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(playTracks(songs, ids, ids[0]))
  }, [book, chapters, dispatch])

  const handleContinue = useCallback(() => {
    if (progress && progress.chapterId) {
      const chapter = chapters.find(c => c.id === progress.chapterId)
      if (chapter) {
        const { songs, ids } = chaptersToSongs(book, chapters)
        dispatch(playTracks(songs, ids, chapter.id, progress.position || 0))
        return
      }
    }
    handlePlayAll()
  }, [progress, chapters, book, dispatch, handlePlayAll])

  const handleAddToQueue = useCallback(() => {
    if (!book || !chapters.length) return
    const { songs, ids } = chaptersToSongs(book, chapters)
    dispatch(addTracks(songs, ids))
  }, [book, chapters, dispatch])

  const handleRescan = async () => {
    setRescanning(true)
    try {
      const res = await fetch(`/api/audiobook/${id}/rescan`, { method: 'POST', headers: getAuthHeaders() })
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

  const handleToggleFavorite = async () => {
    try {
      const method = isStarred ? 'DELETE' : 'POST'
      const res = await fetch(`/api/audiobook/${id}/star`, { method, headers: getAuthHeaders() })
      if (res.ok) {
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

  const handleCoverUpload = async (fileOrUrl) => {
    setCoverUploading(true)
    try {
      const formData = new FormData()
      if (typeof fileOrUrl === 'string') {
        formData.append('url', fileOrUrl)
      } else {
        formData.append('file', fileOrUrl)
      }
      const res = await fetch(`/api/audiobook/${id}/cover`, {
        method: 'POST',
        headers: { 'X-ND-Authorization': `Bearer ${getToken()}` },
        body: formData,
      })
      if (res.ok) {
        const bookRes = await fetch(`/api/audiobook/${id}`, { headers: getAuthHeaders() })
        if (bookRes.ok) {
          const data = await bookRes.json()
          if (data.data) setBook(data.data.book)
        }
        setCoverEditOpen(false)
        setCoverUrl('')
      } else {
        const err = await res.text()
        alert('上传失败: ' + err)
      }
    } catch (err) {
      alert('上传失败: ' + err.message)
    } finally {
      setCoverUploading(false)
    }
  }

  if (loading) {
    return <Box p={4} textAlign="center"><Typography>Loading...</Typography></Box>
  }
  if (!book) {
    return <Box p={4} textAlign="center"><Typography>Audiobook not found</Typography></Box>
  }

  const currentPlayingId = currentPlaying?.id
  const imageUrl = getCoverUrl(book.id)

  // Build detail info string
  const detailParts = []
  if (book.chapterCount > 0) detailParts.push(`${book.chapterCount} 章`)
  if (book.totalDuration > 0) detailParts.push(formatDuration(book.totalDuration))

  return (
    <Box className={classes.root}>
      {/* Top toolbar */}
      <Box className={classes.topBar}>
        <IconButton onClick={onBack || (() => { window.location.hash = '#/audiobook' })} size="small">
          <ArrowBackIcon />
        </IconButton>
        <Box>
          <Tooltip title="重新扫描"><IconButton onClick={handleRescan} disabled={rescanning} size="small"><RefreshIcon fontSize="small" /></IconButton></Tooltip>
          <Tooltip title={isStarred ? '取消收藏' : '收藏'}><IconButton onClick={handleToggleFavorite} size="small">{isStarred ? <FavoriteIcon style={{ color: '#ff4757' }} fontSize="small" /> : <FavoriteBorderIcon fontSize="small" />}</IconButton></Tooltip>
          <Tooltip title="编辑"><IconButton onClick={handleEditOpen} size="small"><EditIcon fontSize="small" /></IconButton></Tooltip>
          <Tooltip title="更换封面"><IconButton onClick={() => setCoverEditOpen(true)} size="small"><span style={{ fontSize: 14 }}>🖼️</span></IconButton></Tooltip>
          <Tooltip title="刮削"><IconButton onClick={() => setScrapeOpen(true)} size="small"><span style={{ fontSize: 14 }}>🔍</span></IconButton></Tooltip>
        </Box>
      </Box>

      {/* Main content card - matching AlbumDetails layout */}
      <Card style={{ boxShadow: 'none', background: 'transparent' }}>
        <div className={classes.cardContents}>
          {/* Cover */}
          <div className={classes.coverParent}>
            {imageUrl ? (
              <CardMedia
                component="img"
                src={imageUrl}
                className={classes.cover}
                title={book.title}
                onError={(e) => { e.target.style.display = 'none' }}
              />
            ) : (
              <Box className={classes.coverPlaceholder}>
                <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
              </Box>
            )}
          </div>

          {/* Info */}
          <div className={classes.details}>
            <CardContent className={classes.content}>
              {/* Title */}
              <Typography variant={isDesktop ? 'h5' : 'h6'} className={classes.recordName}>
                {book.title}
              </Typography>

              {/* Author */}
              {book.author && (
                <Typography className={classes.recordArtist}>
                  <PersonIcon style={{ fontSize: 14, verticalAlign: 'middle', marginRight: 4 }} />
                  {book.author}
                </Typography>
              )}

              {/* Narrator */}
              {book.narrator && (
                <Typography className={classes.recordArtist}>
                  <a href={`#/narrator/${encodeURIComponent(book.narrator)}`} style={{ color: 'inherit', textDecoration: 'none' }}>
                    🎙️ {book.narrator}
                  </a>
                </Typography>
              )}

              {/* Meta info */}
              <Typography component="div" className={classes.recordMeta}>
                {detailParts.join(' · ')}
              </Typography>

              {/* Genre */}
              {book.genre && (
                <Chip label={book.genre} size="small" className={classes.genreChip} />
              )}

              {/* Progress */}
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

              {/* Action buttons */}
              <Box className={classes.actions}>
                <Button variant="contained" color="primary" className={classes.playBtn}
                  startIcon={<PlayArrowIcon />} onClick={handleContinue} size={isXsmall ? 'small' : 'medium'}>
                  {progress ? '继续播放' : '从头播放'}
                </Button>
                <Button variant="outlined" className={classes.playBtn}
                  startIcon={<QueueMusicIcon />} onClick={handleAddToQueue} size={isXsmall ? 'small' : 'medium'}>
                  加入队列
                </Button>
              </Box>
            </CardContent>
          </div>
        </div>

        {/* Description / Notes */}
        {book.description && (
          <Box style={{ padding: '0 16px' }}>
            <Collapse collapsedHeight="3em" in={expandedNotes} timeout="auto">
              <Typography className={classes.notes} variant="body2" onClick={() => setExpandedNotes(!expandedNotes)}>
                {book.description}
              </Typography>
            </Collapse>
          </Box>
        )}
      </Card>

      {/* Chapter list */}
      <Box className={classes.chapterSection}>
        <Typography className={classes.chapterHeader}>
          📑 章节列表 ({chapters.length})
        </Typography>

        {chapters.length === 0 ? (
          <Box textAlign="center" py={4}>
            <Typography color="textSecondary">暂无章节数据，请尝试重新扫描</Typography>
          </Box>
        ) : (
          chapters.map((chapter, idx) => {
            const isActive = currentPlayingId === chapter.id
            return (
              <Box
                key={chapter.id}
                className={`${classes.chapterItem} ${isActive ? classes.chapterActive : ''}`}
                onClick={() => handlePlayChapter(chapter)}
              >
                <Typography className={classes.chapterNum}>
                  {chapter.chapterNumber || idx + 1}
                </Typography>
                <Typography className={classes.chapterTitle}>
                  {chapter.title}
                </Typography>
                {chapter.duration > 0 && (
                  <Typography className={classes.chapterDuration}>
                    {formatDuration(chapter.duration)}
                  </Typography>
                )}
                <Box style={{ marginLeft: 8, flexShrink: 0 }}>
                  {isActive ? (
                    <PauseIcon style={{ fontSize: 18, color: '#1976d2' }} />
                  ) : (
                    <PlayArrowIcon style={{ fontSize: 18, color: '#999' }} />
                  )}
                </Box>
              </Box>
            )
          })
        )}
      </Box>

      {/* Edit Dialog */}
      <Dialog open={editOpen} onClose={() => setEditOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>编辑有声书信息</DialogTitle>
        <DialogContent>
          <TextField label="有声书名称" value={editForm.title || ''} onChange={(e) => setEditForm({ ...editForm, title: e.target.value })} fullWidth margin="normal" />
          <TextField label="作者" value={editForm.author || ''} onChange={(e) => setEditForm({ ...editForm, author: e.target.value })} fullWidth margin="normal" />
          <TextField label="演播者" value={editForm.narrator || ''} onChange={(e) => setEditForm({ ...editForm, narrator: e.target.value })} fullWidth margin="normal" />
          <TextField label="分类" value={editForm.genre || ''} onChange={(e) => setEditForm({ ...editForm, genre: e.target.value })} fullWidth margin="normal" select SelectProps={{ native: true }}>
            <option value="">请选择</option>
            <option value="有声读物">有声读物</option>
            <option value="评书">评书</option>
            <option value="相声">相声</option>
            <option value="戏曲">戏曲</option>
            <option value="儿童">儿童</option>
            <option value="教育">教育</option>
          </TextField>
          <TextField label="系列" value={editForm.series || ''} onChange={(e) => setEditForm({ ...editForm, series: e.target.value })} fullWidth margin="normal" />
          <TextField label="简介" value={editForm.description || ''} onChange={(e) => setEditForm({ ...editForm, description: e.target.value })} fullWidth margin="normal" multiline rows={3} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditOpen(false)}>取消</Button>
          <Button onClick={handleEditSave} color="primary" disabled={saving}>{saving ? '保存中...' : '保存'}</Button>
        </DialogActions>
      </Dialog>

      {/* Cover Edit Dialog */}
      <Dialog open={coverEditOpen} onClose={() => setCoverEditOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>更换封面图片</DialogTitle>
        <DialogContent>
          <Box style={{ display: 'flex', flexDirection: 'column', gap: 16, paddingTop: 8 }}>
            <Typography variant="subtitle2">方式一: 输入图片URL</Typography>
            <TextField label="图片URL" value={coverUrl} onChange={(e) => setCoverUrl(e.target.value)} fullWidth placeholder="https://example.com/cover.jpg" variant="outlined" size="small" />
            <Button variant="contained" color="primary" disabled={!coverUrl.trim() || coverUploading} onClick={() => handleCoverUpload(coverUrl.trim())}>
              {coverUploading ? '上传中...' : '从URL上传'}
            </Button>
            <Typography variant="subtitle2" style={{ marginTop: 8 }}>方式二: 选择本地文件</Typography>
            <input type="file" accept="image/*" onChange={(e) => { if (e.target.files[0]) handleCoverUpload(e.target.files[0]) }} style={{ fontSize: 14 }} />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setCoverEditOpen(false); setCoverUrl('') }}>取消</Button>
        </DialogActions>
      </Dialog>

      {/* Scrape Dialog */}
      <ScrapeDialog open={scrapeOpen} onClose={() => setScrapeOpen(false)} book={book} onApply={() => {
        fetch(`/api/audiobook/${id}`, { headers: getAuthHeaders() })
          .then(res => res.json())
          .then(data => { if (data.data) { setBook(data.data.book); setChapters(data.data.chapters || []) } })
          .catch(() => {})
      }} />
    </Box>
  )
}

export default AudiobookDetail
