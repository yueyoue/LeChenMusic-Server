import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, IconButton,
  Chip, Button, LinearProgress, Tooltip
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

  useEffect(() => {
    const fetchData = async () => {
      try {
        const token = localStorage.getItem('token')
        const headers = { 'X-ND-Authorization': `Bearer ${token}` }
        const [bookRes, progressRes] = await Promise.all([
          fetch(`/api/audiobook/${id}`, { headers }),
          fetch(`/api/audiobook/${id}/progress`, { headers }),
        ])
        if (bookRes.ok) {
          const data = await bookRes.json()
          if (data.data) {
            setBook(data.data.book)
            setChapters(data.data.chapters || [])
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

  if (loading) {
    return <Box p={4} textAlign="center"><Typography>Loading...</Typography></Box>
  }

  if (!book) {
    return <Box p={4} textAlign="center"><Typography>Audiobook not found</Typography></Box>
  }

  return (
    <Box className={classes.root}>
      <IconButton className={classes.backBtn} onClick={onBack}>
        <ArrowBackIcon />
      </IconButton>

      <Box className={classes.header}>
        {book.coverPath ? (
          <img src={`/api/audiobook/${book.id}/cover`} alt={book.title} className={classes.cover} />
        ) : (
          <Box className={classes.coverPlaceholder}>
            <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
          </Box>
        )}
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
    </Box>
  )
}

export default AudiobookDetail
