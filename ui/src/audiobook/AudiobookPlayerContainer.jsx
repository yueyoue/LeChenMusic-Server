import React, { useState, useEffect, useRef, useCallback } from 'react'
import AudiobookPlayer from './AudiobookPlayer'

/**
 * AudiobookPlayerContainer - Global container that listens for audiobook-play events
 * and manages the audiobook player state.
 */
const AudiobookPlayerContainer = () => {
  const [book, setBook] = useState(null)
  const [chapters, setChapters] = useState([])
  const [currentChapter, setCurrentChapter] = useState(null)
  const [isPlaying, setIsPlaying] = useState(false)
  const [position, setPosition] = useState(0)
  const [duration, setDuration] = useState(0)
  const [playbackSpeed, setPlaybackSpeed] = useState(1.0)
  const [isBookmarked, setIsBookmarked] = useState(false)
  const [skipIntro, setSkipIntro] = useState(0)
  const [skipOutro, setSkipOutro] = useState(0)

  const audioRef = useRef(null)
  const progressTimerRef = useRef(null)
  const getToken = () => localStorage.getItem('token')

  // Listen for audiobook-play events
  useEffect(() => {
    const handlePlay = (e) => {
      const { book: newBook, chapters: newChapters, chapter } = e.detail
      setBook(newBook)
      setChapters(newChapters)
      setCurrentChapter(chapter)
      setIsPlaying(true)
      setPosition(0)

      // Build stream URL
      const token = getToken()
      const streamUrl = `/api/audiobook/${newBook.id}/chapters/${chapter.id}/stream${token ? '?token=' + token : ''}`

      // Create or update audio element
      if (!audioRef.current) {
        audioRef.current = new Audio()
        audioRef.current.addEventListener('timeupdate', () => {
          setPosition(audioRef.current.currentTime * 1000)
        })
        audioRef.current.addEventListener('loadedmetadata', () => {
          setDuration(audioRef.current.duration * 1000)
        })
        audioRef.current.addEventListener('ended', () => {
          // Auto-play next chapter
          const idx = newChapters.findIndex(c => c.id === chapter.id)
          if (idx < newChapters.length - 1) {
            const nextChapter = newChapters[idx + 1]
            setCurrentChapter(nextChapter)
            const nextUrl = `/api/audiobook/${newBook.id}/chapters/${nextChapter.id}/stream${token ? '?token=' + token : ''}`
            audioRef.current.src = nextUrl
            audioRef.current.play()
          } else {
            setIsPlaying(false)
          }
        })
      }

      audioRef.current.src = streamUrl
      audioRef.current.playbackRate = playbackSpeed
      audioRef.current.play().catch(err => {
        console.error('Failed to play audiobook:', err)
        setIsPlaying(false)
      })

      // Save progress periodically
      if (progressTimerRef.current) {
        clearInterval(progressTimerRef.current)
      }
      progressTimerRef.current = setInterval(() => {
        if (audioRef.current && !audioRef.current.paused) {
          saveProgress(newBook.id, chapter.id, chapter.chapterNumber, Math.floor(audioRef.current.currentTime))
        }
      }, 30000) // Save every 30 seconds
    }

    window.addEventListener('audiobook-play', handlePlay)
    return () => {
      window.removeEventListener('audiobook-play', handlePlay)
      if (progressTimerRef.current) {
        clearInterval(progressTimerRef.current)
      }
    }
  }, [playbackSpeed])

  // Save progress to server
  const saveProgress = async (bookId, chapterId, chapterNumber, positionSeconds) => {
    try {
      const token = getToken()
      await fetch(`/api/audiobook/${bookId}/progress`, {
        method: 'PUT',
        headers: {
          'X-ND-Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          chapterId,
          chapterNumber,
          position: positionSeconds,
          playbackSpeed,
          skipIntro,
          skipOutro,
        }),
      })
    } catch (err) {
      console.error('Failed to save progress:', err)
    }
  }

  const handlePlayPause = useCallback(() => {
    if (!audioRef.current) return
    if (isPlaying) {
      audioRef.current.pause()
      setIsPlaying(false)
    } else {
      audioRef.current.play()
      setIsPlaying(true)
    }
  }, [isPlaying])

  const handleSeek = useCallback((newPosition) => {
    if (!audioRef.current) return
    audioRef.current.currentTime = newPosition / 1000
    setPosition(newPosition)
  }, [])

  const handleSkipForward = useCallback(() => {
    if (!audioRef.current) return
    audioRef.current.currentTime += 15
  }, [])

  const handleSkipBack = useCallback(() => {
    if (!audioRef.current) return
    audioRef.current.currentTime = Math.max(0, audioRef.current.currentTime - 15)
  }, [])

  const handlePrevChapter = useCallback(() => {
    if (!book || !chapters.length || !currentChapter) return
    const idx = chapters.findIndex(c => c.id === currentChapter.id)
    if (idx > 0) {
      const prevChapter = chapters[idx - 1]
      setCurrentChapter(prevChapter)
      const token = getToken()
      const url = `/api/audiobook/${book.id}/chapters/${prevChapter.id}/stream${token ? '?token=' + token : ''}`
      audioRef.current.src = url
      audioRef.current.playbackRate = playbackSpeed
      audioRef.current.play()
      setIsPlaying(true)
      setPosition(0)
    }
  }, [book, chapters, currentChapter, playbackSpeed])

  const handleNextChapter = useCallback(() => {
    if (!book || !chapters.length || !currentChapter) return
    const idx = chapters.findIndex(c => c.id === currentChapter.id)
    if (idx < chapters.length - 1) {
      const nextChapter = chapters[idx + 1]
      setCurrentChapter(nextChapter)
      const token = getToken()
      const url = `/api/audiobook/${book.id}/chapters/${nextChapter.id}/stream${token ? '?token=' + token : ''}`}
      audioRef.current.src = url
      audioRef.current.playbackRate = playbackSpeed
      audioRef.current.play()
      setIsPlaying(true)
      setPosition(0)
    }
  }, [book, chapters, currentChapter, playbackSpeed])

  const handleChapterSelect = useCallback((chapter) => {
    if (!book) return
    setCurrentChapter(chapter)
    const token = getToken()
    const url = `/api/audiobook/${book.id}/chapters/${chapter.id}/stream${token ? '?token=' + token : ''}`
    audioRef.current.src = url
    audioRef.current.playbackRate = playbackSpeed
    audioRef.current.play()
    setIsPlaying(true)
    setPosition(0)
  }, [book, playbackSpeed])

  const handleSpeedChange = useCallback((speed) => {
    setPlaybackSpeed(speed)
    if (audioRef.current) {
      audioRef.current.playbackRate = speed
    }
  }, [])

  const handleBack = useCallback(() => {
    // Save progress before closing
    if (book && currentChapter && audioRef.current) {
      saveProgress(book.id, currentChapter.id, currentChapter.chapterNumber, Math.floor(audioRef.current.currentTime))
    }
    // Stop playback
    if (audioRef.current) {
      audioRef.current.pause()
      audioRef.current = null
    }
    setIsPlaying(false)
    setBook(null)
    setChapters([])
    setCurrentChapter(null)
  }, [book, currentChapter])

  const handleBookmark = useCallback(async () => {
    if (!book || !currentChapter || !audioRef.current) return
    try {
      const token = getToken()
      const method = isBookmarked ? 'DELETE' : 'POST'
      await fetch(`/api/audiobook/${book.id}/bookmarks`, {
        method: 'POST',
        headers: {
          'X-ND-Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          chapterId: currentChapter.id,
          position: Math.floor(audioRef.current.currentTime),
          title: `${currentChapter.title} - ${formatTime(audioRef.current.currentTime * 1000)}`,
        }),
      })
      setIsBookmarked(!isBookmarked)
    } catch (err) {
      console.error('Failed to save bookmark:', err)
    }
  }, [book, currentChapter, isBookmarked])

  const handleSkipChange = useCallback((type, value) => {
    if (type === 'intro') {
      setSkipIntro(value)
    } else {
      setSkipOutro(value)
    }
  }, [])

  // Don't render if no book is loaded
  if (!book || !currentChapter) {
    return null
  }

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      zIndex: 1000,
      background: 'rgba(0,0,0,0.95)',
    }}>
      <AudiobookPlayer
        book={book}
        chapters={chapters}
        currentChapter={currentChapter}
        isPlaying={isPlaying}
        position={position}
        duration={duration}
        playbackSpeed={playbackSpeed}
        onPlay={handlePlayPause}
        onPause={handlePlayPause}
        onSeek={handleSeek}
        onSkipForward={handleSkipForward}
        onSkipBack={handleSkipBack}
        onPrevChapter={handlePrevChapter}
        onNextChapter={handleNextChapter}
        onChapterSelect={handleChapterSelect}
        onSpeedChange={handleSpeedChange}
        onBack={handleBack}
        onBookmark={handleBookmark}
        isBookmarked={isBookmarked}
        skipIntro={skipIntro}
        skipOutro={skipOutro}
        onSkipChange={handleSkipChange}
      />
    </div>
  )
}

const formatTime = (ms) => {
  if (!ms || ms < 0) return '0:00'
  const s = Math.floor(ms / 1000)
  const m = Math.floor(s / 60)
  const h = Math.floor(m / 60)
  const sec = s % 60
  if (h > 0) return `${h}:${String(m % 60).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
  return `${m}:${String(sec).padStart(2, '0')}`
}

export default AudiobookPlayerContainer
