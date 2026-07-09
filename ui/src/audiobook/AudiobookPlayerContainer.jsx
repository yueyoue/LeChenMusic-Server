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

  useEffect(() => {
    const handlePlay = (e) => {
      const { book: newBook, chapters: newChapters, chapter } = e.detail
      setBook(newBook)
      setChapters(newChapters)
      setCurrentChapter(chapter)
      setIsPlaying(true)
      setPosition(0)
      const token = getToken()
      const streamUrl = `/api/audiobook/${newBook.id}/chapters/${chapter.id}/stream${token ? '?token=' + token : ''}`
      if (!audioRef.current) {
        audioRef.current = new Audio()
        audioRef.current.addEventListener('timeupdate', () => setPosition(audioRef.current.currentTime * 1000))
        audioRef.current.addEventListener('loadedmetadata', () => setDuration(audioRef.current.duration * 1000))
        audioRef.current.addEventListener('ended', () => {
          const idx = newChapters.findIndex(c => c.id === chapter.id)
          if (idx < newChapters.length - 1) {
            const next = newChapters[idx + 1]
            setCurrentChapter(next)
            const nextToken = getToken()
            audioRef.current.src = `/api/audiobook/${newBook.id}/chapters/${next.id}/stream${nextToken ? '?token=' + nextToken : ''}`
            audioRef.current.play()
          } else {
            setIsPlaying(false)
          }
        })
      }
      audioRef.current.src = streamUrl
      audioRef.current.playbackRate = playbackSpeed
      audioRef.current.play().catch(err => {
        console.error('Play failed:', err)
        setIsPlaying(false)
      })
      if (progressTimerRef.current) clearInterval(progressTimerRef.current)
      progressTimerRef.current = setInterval(() => {
        if (audioRef.current && !audioRef.current.paused) {
          saveProgress(newBook.id, chapter.id, chapter.chapterNumber, Math.floor(audioRef.current.currentTime))
        }
      }, 30000)
    }
    window.addEventListener('audiobook-play', handlePlay)
    return () => {
      window.removeEventListener('audiobook-play', handlePlay)
      if (progressTimerRef.current) clearInterval(progressTimerRef.current)
    }
  }, [playbackSpeed])

  const saveProgress = async (bookId, chapterId, chapterNumber, pos) => {
    try {
      const token = getToken()
      await fetch(`/api/audiobook/${bookId}/progress`, {
        method: 'PUT',
        headers: { 'X-ND-Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ chapterId, chapterNumber, position: pos, playbackSpeed, skipIntro, skipOutro })
      })
    } catch (e) {}
  }

  const handlePlayPause = useCallback(() => {
    if (!audioRef.current) return
    if (isPlaying) { audioRef.current.pause(); setIsPlaying(false) }
    else { audioRef.current.play(); setIsPlaying(true) }
  }, [isPlaying])

  const handleSeek = useCallback((p) => { if (audioRef.current) { audioRef.current.currentTime = p / 1000; setPosition(p) } }, [])
  const handleSkipForward = useCallback(() => { if (audioRef.current) audioRef.current.currentTime += 15 }, [])
  const handleSkipBack = useCallback(() => { if (audioRef.current) audioRef.current.currentTime = Math.max(0, audioRef.current.currentTime - 15) }, [])

  const playChapter = (chapter) => {
    if (!book) return
    setCurrentChapter(chapter)
    const token = getToken()
    audioRef.current.src = `/api/audiobook/${book.id}/chapters/${chapter.id}/stream${token ? '?token=' + token : ''}`
    audioRef.current.playbackRate = playbackSpeed
    audioRef.current.play()
    setIsPlaying(true)
    setPosition(0)
  }

  const handlePrevChapter = useCallback(() => {
    if (!book || !chapters.length || !currentChapter) return
    const idx = chapters.findIndex(c => c.id === currentChapter.id)
    if (idx > 0) playChapter(chapters[idx - 1])
  }, [book, chapters, currentChapter, playbackSpeed])

  const handleNextChapter = useCallback(() => {
    if (!book || !chapters.length || !currentChapter) return
    const idx = chapters.findIndex(c => c.id === currentChapter.id)
    if (idx < chapters.length - 1) playChapter(chapters[idx + 1])
  }, [book, chapters, currentChapter, playbackSpeed])

  const handleChapterSelect = useCallback((chapter) => playChapter(chapter), [book, playbackSpeed])
  const handleSpeedChange = useCallback((speed) => { setPlaybackSpeed(speed); if (audioRef.current) audioRef.current.playbackRate = speed }, [])

  const handleBack = useCallback(() => {
    if (book && currentChapter && audioRef.current) saveProgress(book.id, currentChapter.id, currentChapter.chapterNumber, Math.floor(audioRef.current.currentTime))
    if (audioRef.current) { audioRef.current.pause(); audioRef.current = null }
    setIsPlaying(false); setBook(null); setChapters([]); setCurrentChapter(null)
  }, [book, currentChapter])

  const handleBookmark = useCallback(async () => {
    if (!book || !currentChapter || !audioRef.current) return
    try {
      const token = getToken()
      await fetch(`/api/audiobook/${book.id}/bookmarks`, {
        method: 'POST',
        headers: { 'X-ND-Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ chapterId: currentChapter.id, position: Math.floor(audioRef.current.currentTime), title: currentChapter.title })
      })
      setIsBookmarked(!isBookmarked)
    } catch (e) {}
  }, [book, currentChapter, isBookmarked])

  const handleSkipChange = useCallback((type, value) => { if (type === 'intro') setSkipIntro(value); else setSkipOutro(value) }, [])

  if (!book || !currentChapter) return null

  return (
    <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 1000, background: 'rgba(0,0,0,0.95)' }}>
      <AudiobookPlayer
        book={book} chapters={chapters} currentChapter={currentChapter} isPlaying={isPlaying}
        position={position} duration={duration} playbackSpeed={playbackSpeed}
        onPlay={handlePlayPause} onPause={handlePlayPause} onSeek={handleSeek}
        onSkipForward={handleSkipForward} onSkipBack={handleSkipBack}
        onPrevChapter={handlePrevChapter} onNextChapter={handleNextChapter}
        onChapterSelect={handleChapterSelect} onSpeedChange={handleSpeedChange}
        onBack={handleBack} onBookmark={handleBookmark} isBookmarked={isBookmarked}
        skipIntro={skipIntro} skipOutro={skipOutro} onSkipChange={handleSkipChange}
      />
    </div>
  )
}

export default AudiobookPlayerContainer
