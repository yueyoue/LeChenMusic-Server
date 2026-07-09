import React, { useState, useEffect, useRef, useCallback } from 'react'
import {
  Typography, Box, makeStyles, IconButton, Slider, Chip,
  Tooltip, Menu, MenuItem, Dialog, DialogTitle, DialogContent,
  List, ListItem, ListItemText, ListItemSecondaryAction, Switch,
  TextField, DialogActions, Button
} from '@material-ui/core'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PauseIcon from '@material-ui/icons/Pause'
import SkipPreviousIcon from '@material-ui/icons/SkipPrevious'
import SkipNextIcon from '@material-ui/icons/SkipNext'
import Replay10Icon from '@material-ui/icons/Replay10'
import Forward10Icon from '@material-ui/icons/Forward10'
import SpeedIcon from '@material-ui/icons/Speed'
import BookmarkBorderIcon from '@material-ui/icons/BookmarkBorder'
import BookmarkIcon from '@material-ui/icons/Bookmark'
import TimerIcon from '@material-ui/icons/Timer'
import ListIcon from '@material-ui/icons/List'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import SkipNext from '@material-ui/icons/SkipNext'

const useStyles = makeStyles((theme) => ({
  root: {
    display: 'flex', flexDirection: 'column', height: '100%',
    background: theme.palette.background.default,
  },
  topBar: {
    display: 'flex', alignItems: 'center', padding: '12px 16px',
    justifyContent: 'space-between',
  },
  coverWrap: {
    flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center',
    padding: '0 40px',
  },
  cover: {
    width: 260, height: 340, borderRadius: 12, objectFit: 'cover',
    boxShadow: '0 8px 30px rgba(0,0,0,0.2)',
  },
  coverPlaceholder: {
    width: 260, height: 340, borderRadius: 12,
    backgroundColor: theme.palette.grey[200],
    display: 'flex', alignItems: 'center', justifyContent: 'center',
  },
  info: {
    textAlign: 'center', padding: '20px 24px 0',
  },
  bookTitle: { fontSize: 18, fontWeight: 700, marginBottom: 4 },
  chapterTitle: { fontSize: 14, color: theme.palette.text.secondary, marginBottom: 4 },
  chapterCounter: { fontSize: 12, color: theme.palette.text.secondary },
  progress: { padding: '20px 24px 0' },
  timeRow: { display: 'flex', justifyContent: 'space-between', marginTop: 4 },
  timeText: { fontSize: 11, color: theme.palette.text.secondary },
  controls: {
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    gap: 20, padding: '20px 24px 0',
  },
  playBtn: {
    width: 64, height: 64, borderRadius: 32,
    backgroundColor: theme.palette.primary.main,
    color: '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center',
    cursor: 'pointer', border: 'none', fontSize: 28,
    '&:hover': { opacity: 0.9 },
  },
  skipBtn: {
    width: 44, height: 44, borderRadius: 22,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    cursor: 'pointer', border: 'none', background: 'transparent',
    color: theme.palette.text.primary, fontSize: 18,
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  navBtn: {
    width: 40, height: 40, display: 'flex', alignItems: 'center',
    justifyContent: 'center', cursor: 'pointer', border: 'none',
    background: 'transparent', color: theme.palette.text.secondary,
    '&:hover': { color: theme.palette.text.primary },
  },
  extra: {
    display: 'flex', justifyContent: 'space-around', padding: '16px 24px 24px',
  },
  extraBtn: {
    display: 'flex', flexDirection: 'column', alignItems: 'center',
    gap: 4, cursor: 'pointer', border: 'none', background: 'transparent',
    color: theme.palette.text.secondary, fontSize: 11,
    '&:hover': { color: theme.palette.text.primary },
  },
  extraBtnActive: {
    color: theme.palette.primary.main,
  },
  chapterDrawer: {
    position: 'absolute', bottom: 0, left: 0, right: 0,
    maxHeight: '60%', backgroundColor: theme.palette.background.paper,
    borderTopLeftRadius: 16, borderTopRightRadius: 16,
    boxShadow: '0 -4px 20px rgba(0,0,0,0.15)', overflow: 'auto',
    zIndex: 10,
  },
  chapterItem: {
    display: 'flex', alignItems: 'center', padding: '10px 16px',
    cursor: 'pointer', '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  chapterActive: { backgroundColor: theme.palette.action.selected },
  speedMenu: { minWidth: 80 },
  timerChip: {
    position: 'absolute', top: -8, right: -8,
    fontSize: 9, height: 16,
  },
  skipSettings: {
    padding: '12px 16px',
    borderTop: '1px solid rgba(128,128,128,0.1)',
  },
}))

const SPEEDS = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 2.5, 3.0]
const TIMER_OPTIONS = [
  { label: '关闭', minutes: 0 },
  { label: '15 分钟', minutes: 15 },
  { label: '30 分钟', minutes: 30 },
  { label: '45 分钟', minutes: 45 },
  { label: '60 分钟', minutes: 60 },
  { label: '90 分钟', minutes: 90 },
  { label: '本章结束', minutes: -1 },
]

const formatTime = (ms) => {
  if (!ms || ms < 0) return '0:00'
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  const sec = s % 60
  if (h > 0) return `${h}:${String(m % 60).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
  return `${m}:${String(sec).padStart(2, '0')}`
}

const AudiobookPlayer = ({
  book, chapters, currentChapter, isPlaying, position, duration,
  playbackSpeed, onPlay, onPause, onSeek, onSkipForward, onSkipBack,
  onPrevChapter, onNextChapter, onChapterSelect, onSpeedChange,
  onBack, onBookmark, isBookmarked, skipIntro, skipOutro, onSkipChange,
}) => {
  const classes = useStyles()
  const [showChapters, setShowChapters] = useState(false)
  const [showSpeed, setShowSpeed] = useState(null)
  const [showTimer, setShowTimer] = useState(false)
  const [showSkipSettings, setShowSkipSettings] = useState(false)
  const [isDragging, setIsDragging] = useState(false)
  const [dragValue, setDragValue] = useState(0)
  const [timerMinutes, setTimerMinutes] = useState(0)
  const [timerRemaining, setTimerRemaining] = useState(0)
  const timerRef = useRef(null)

  const currentIdx = chapters.findIndex(c => c.id === currentChapter?.id)
  const progress = duration > 0 ? (position / duration) * 100 : 0

  // Sleep timer
  useEffect(() => {
    if (timerMinutes === 0) {
      setTimerRemaining(0)
      if (timerRef.current) clearInterval(timerRef.current)
      return
    }

    if (timerMinutes === -1) {
      // Stop at end of chapter
      setTimerRemaining(-1)
      return
    }

    setTimerRemaining(timerMinutes * 60)
    timerRef.current = setInterval(() => {
      setTimerRemaining(prev => {
        if (prev <= 1) {
          clearInterval(timerRef.current)
          if (onPause) onPause()
          setTimerMinutes(0)
          return 0
        }
        return prev - 1
      })
    }, 1000)

    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [timerMinutes])

  // Check chapter end for "stop at chapter end" timer
  useEffect(() => {
    if (timerMinutes === -1 && isPlaying && duration > 0) {
      const remaining = duration - position
      if (remaining < 2000) {
        if (onPause) onPause()
        setTimerMinutes(0)
      }
    }
  }, [position, duration, timerMinutes, isPlaying])

  const handleSliderChange = (_, val) => {
    setIsDragging(true)
    setDragValue(val)
  }

  const handleSliderCommitted = (_, val) => {
    setIsDragging(false)
    onSeek((val / 100) * duration)
  }

  const formatTimerRemaining = () => {
    if (timerMinutes === -1) return '本章结束'
    if (timerMinutes === 0) return '定时'
    const m = Math.floor(timerRemaining / 60)
    const s = timerRemaining % 60
    return `${m}:${String(s).padStart(2, '0')}`
  }

  return (
    <Box className={classes.root}>
      {/* Top bar */}
      <Box className={classes.topBar}>
        <IconButton onClick={onBack} size="small"><ArrowBackIcon /></IconButton>
        <Typography style={{ fontSize: 14, fontWeight: 600 }}>正在播放</Typography>
        <Box style={{ width: 40 }} />
      </Box>

      {/* Cover */}
      <Box className={classes.coverWrap}>
        {book?.coverPath ? (
          <img src={`/api/audiobook/${book.id}/cover`} alt={book.title} className={classes.cover} />
        ) : (
          <Box className={classes.coverPlaceholder}>
            <MenuBookIcon style={{ fontSize: 64, opacity: 0.2 }} />
          </Box>
        )}
      </Box>

      {/* Info */}
      <Box className={classes.info}>
        <Typography className={classes.bookTitle}>{book?.title}</Typography>
        <Typography className={classes.chapterTitle}>{currentChapter?.title}</Typography>
        <Typography className={classes.chapterCounter}>
          第 {currentIdx + 1} / {chapters.length} 章
        </Typography>
      </Box>

      {/* Progress */}
      <Box className={classes.progress}>
        <Slider
          value={isDragging ? dragValue : progress}
          onChange={handleSliderChange}
          onChangeCommitted={handleSliderCommitted}
          min={0} max={100} step={0.1}
          style={{ color: 'var(--primary, #1976d2)' }}
        />
        <Box className={classes.timeRow}>
          <Typography className={classes.timeText}>{formatTime(position)}</Typography>
          <Typography className={classes.timeText}>{formatTime(duration)}</Typography>
        </Box>
      </Box>

      {/* Controls */}
      <Box className={classes.controls}>
        <IconButton className={classes.navBtn} onClick={onPrevChapter} disabled={currentIdx <= 0}>
          <SkipPreviousIcon />
        </IconButton>
        <IconButton className={classes.skipBtn} onClick={onSkipBack} title="后退15秒">
          <Replay10Icon style={{ fontSize: 28 }} />
          <span style={{ fontSize: 9, position: 'absolute', bottom: 4 }}>15</span>
        </IconButton>
        <button className={classes.playBtn} onClick={isPlaying ? onPause : onPlay}>
          {isPlaying ? <PauseIcon style={{ fontSize: 32 }} /> : <PlayArrowIcon style={{ fontSize: 32 }} />}
        </button>
        <IconButton className={classes.skipBtn} onClick={onSkipForward} title="前进15秒">
          <Forward10Icon style={{ fontSize: 28 }} />
          <span style={{ fontSize: 9, position: 'absolute', bottom: 4 }}>15</span>
        </IconButton>
        <IconButton className={classes.navBtn} onClick={onNextChapter} disabled={currentIdx >= chapters.length - 1}>
          <SkipNextIcon />
        </IconButton>
      </Box>

      {/* Extra controls */}
      <Box className={classes.extra}>
        <button className={classes.extraBtn} onClick={(e) => setShowSpeed(e.currentTarget)}>
          <SpeedIcon style={{ fontSize: 20 }} />
          <span>{playbackSpeed}x</span>
        </button>
        <button className={classes.extraBtn} onClick={onBookmark}>
          {isBookmarked ? <BookmarkIcon style={{ fontSize: 20, color: '#f59e0b' }} /> : <BookmarkBorderIcon style={{ fontSize: 20 }} />}
          <span>书签</span>
        </button>
        <button
          className={`${classes.extraBtn} ${timerMinutes !== 0 ? classes.extraBtnActive : ''}`}
          onClick={() => setShowTimer(true)}
          style={{ position: 'relative' }}
        >
          <TimerIcon style={{ fontSize: 20 }} />
          <span>{formatTimerRemaining()}</span>
          {timerMinutes !== 0 && <Chip label="ON" size="small" color="primary" className={classes.timerChip} />}
        </button>
        <button className={classes.extraBtn} onClick={() => setShowSkipSettings(!showSkipSettings)}>
          <SkipNext style={{ fontSize: 20 }} />
          <span>跳过</span>
        </button>
        <button className={classes.extraBtn} onClick={() => setShowChapters(!showChapters)}>
          <ListIcon style={{ fontSize: 20 }} />
          <span>章节</span>
        </button>
      </Box>

      {/* Skip intro/outro settings */}
      {showSkipSettings && (
        <Box className={classes.skipSettings}>
          <Typography style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>跳过片头片尾（秒）</Typography>
          <Box display="flex" gap={2} alignItems="center">
            <Box flex={1}>
              <Typography style={{ fontSize: 11, color: 'text.secondary', marginBottom: 4 }}>片头</Typography>
              <TextField
                type="number" size="small" variant="outlined" fullWidth
                value={skipIntro || 0}
                onChange={(e) => onSkipChange && onSkipChange('intro', parseInt(e.target.value) || 0)}
                inputProps={{ min: 0, max: 300 }}
              />
            </Box>
            <Box flex={1}>
              <Typography style={{ fontSize: 11, color: 'text.secondary', marginBottom: 4 }}>片尾</Typography>
              <TextField
                type="number" size="small" variant="outlined" fullWidth
                value={skipOutro || 0}
                onChange={(e) => onSkipChange && onSkipChange('outro', parseInt(e.target.value) || 0)}
                inputProps={{ min: 0, max: 300 }}
              />
            </Box>
          </Box>
        </Box>
      )}

      {/* Speed menu */}
      <Menu anchorEl={showSpeed} open={Boolean(showSpeed)} onClose={() => setShowSpeed(null)}>
        {SPEEDS.map(s => (
          <MenuItem key={s} selected={playbackSpeed === s} onClick={() => { onSpeedChange(s); setShowSpeed(null) }}>
            {s}x {s === 1.0 && '(正常)'}
          </MenuItem>
        ))}
      </Menu>

      {/* Timer dialog */}
      <Dialog open={showTimer} onClose={() => setShowTimer(false)} maxWidth="xs" fullWidth>
        <DialogTitle>睡眠定时器</DialogTitle>
        <DialogContent>
          <List>
            {TIMER_OPTIONS.map(opt => (
              <ListItem
                key={opt.minutes}
                button
                selected={timerMinutes === opt.minutes}
                onClick={() => { setTimerMinutes(opt.minutes); setShowTimer(false) }}
              >
                <ListItemText primary={opt.label} />
                {timerMinutes === opt.minutes && (
                  <ListItemSecondaryAction>
                    <Typography style={{ fontSize: 12, color: 'primary.main' }}>✓</Typography>
                  </ListItemSecondaryAction>
                )}
              </ListItem>
            ))}
          </List>
        </DialogContent>
      </Dialog>

      {/* Chapter drawer */}
      {showChapters && (
        <Box className={classes.chapterDrawer}>
          <Box style={{ padding: '12px 16px', borderBottom: '1px solid rgba(128,128,128,0.2)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography style={{ fontWeight: 600 }}>章节列表 ({chapters.length})</Typography>
            <IconButton size="small" onClick={() => setShowChapters(false)}>✕</IconButton>
          </Box>
          {chapters.map((ch, idx) => (
            <Box
              key={ch.id}
              className={`${classes.chapterItem} ${currentChapter?.id === ch.id ? classes.chapterActive : ''}`}
              onClick={() => { onChapterSelect(ch); setShowChapters(false) }}
            >
              <Typography style={{ width: 32, fontSize: 12, color: 'text.secondary', textAlign: 'center' }}>
                {ch.chapterNumber || idx + 1}
              </Typography>
              <Typography style={{ flex: 1, fontSize: 13, marginLeft: 8 }} noWrap>{ch.title}</Typography>
              {currentChapter?.id === ch.id && (
                <Typography style={{ fontSize: 11, color: 'primary.main' }}>正在播放</Typography>
              )}
            </Box>
          ))}
        </Box>
      )}
    </Box>
  )
}

export default AudiobookPlayer
