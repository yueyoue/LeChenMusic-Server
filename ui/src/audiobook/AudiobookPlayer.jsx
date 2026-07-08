import React, { useState, useEffect, useRef, useCallback } from 'react'
import {
  Typography, Box, makeStyles, IconButton, Slider, Chip,
  Tooltip, Menu, MenuItem
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
}))

const SPEEDS = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0, 2.5, 3.0]

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
  onBack, onBookmark, isBookmarked,
}) => {
  const classes = useStyles()
  const [showChapters, setShowChapters] = useState(false)
  const [showSpeed, setShowSpeed] = useState(null)
  const [isDragging, setIsDragging] = useState(false)
  const [dragValue, setDragValue] = useState(0)

  const currentIdx = chapters.findIndex(c => c.id === currentChapter?.id)
  const progress = duration > 0 ? (position / duration) * 100 : 0

  const handleSliderChange = (_, val) => {
    setIsDragging(true)
    setDragValue(val)
  }

  const handleSliderCommitted = (_, val) => {
    setIsDragging(false)
    onSeek((val / 100) * duration)
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
        <button className={classes.extraBtn}>
          <TimerIcon style={{ fontSize: 20 }} />
          <span>定时</span>
        </button>
        <button className={classes.extraBtn} onClick={() => setShowChapters(!showChapters)}>
          <ListIcon style={{ fontSize: 20 }} />
          <span>章节</span>
        </button>
      </Box>

      {/* Speed menu */}
      <Menu anchorEl={showSpeed} open={Boolean(showSpeed)} onClose={() => setShowSpeed(null)}>
        {SPEEDS.map(s => (
          <MenuItem key={s} selected={playbackSpeed === s} onClick={() => { onSpeedChange(s); setShowSpeed(null) }}>
            {s}x {s === 1.0 && '(正常)'}
          </MenuItem>
        ))}
      </Menu>

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
