import React, { useState, useEffect } from 'react'
import {
  useTranslate,
  useRefresh,
} from 'react-admin'
import {
  Card,
  CardContent,
  Typography,
  Box,
  Chip,
  makeStyles,
  IconButton,
} from '@material-ui/core'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import { useLocation } from 'react-router-dom'

const useStyles = makeStyles((theme) => ({
  card: {
    marginBottom: 8,
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  cover: {
    width: 60,
    height: 60,
    borderRadius: 4,
    objectFit: 'cover',
    backgroundColor: theme.palette.grey[300],
  },
  title: {
    fontWeight: 600,
    fontSize: 14,
  },
  sub: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },
  genre: {
    fontSize: 11,
    marginRight: 4,
  },
  empty: {
    textAlign: 'center',
    padding: 40,
    color: theme.palette.text.secondary,
  },
}))

const AudiobookList = () => {
  const classes = useStyles()
  const translate = useTranslate()
  const location = useLocation()
  const [audiobooks, setAudiobooks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // Parse genre from URL query
  const params = new URLSearchParams(location.search)
  const genreFilter = params.get('genre')
  const isStarred = location.pathname.includes('/starred')

  useEffect(() => {
    const fetchAudiobooks = async () => {
      try {
        const token = localStorage.getItem('token')
        let url = '/api/audiobook'
        if (isStarred) {
          url = '/api/audiobook/starred'
        }
        const response = await fetch(url, {
          headers: {
            'X-ND-Authorization': `Bearer ${token}`,
          },
        })
        if (!response.ok) {
          throw new Error('Failed to fetch')
        }
        const data = await response.json()
        let books = data.data || []
        // Filter by genre if specified
        if (genreFilter) {
          books = books.filter((b) => b.genre === genreFilter)
        }
        setAudiobooks(books)
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchAudiobooks()
  }, [genreFilter, isStarred])

  const getTitle = () => {
    if (isStarred) return '⭐ 收藏的有声书'
    if (genreFilter) return `📖 ${genreFilter}`
    return '📖 全部有声书'
  }

  if (loading) {
    return (
      <Box p={2} textAlign="center">
        <Typography>{translate('ra.loading')}...</Typography>
      </Box>
    )
  }

  if (error) {
    return (
      <Box p={2} textAlign="center">
        <Typography color="error">{error}</Typography>
      </Box>
    )
  }

  if (audiobooks.length === 0) {
    return (
      <Box className={classes.empty}>
        <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
        <Typography style={{ marginTop: 8 }}>
          {genreFilter ? `暂无${genreFilter}` : '暂无有声书'}
        </Typography>
      </Box>
    )
  }

  return (
    <Box p={1}>
      <Typography variant="h6" style={{ padding: '8px 16px', fontWeight: 600 }}>
        {getTitle()} ({audiobooks.length})
      </Typography>
      {audiobooks.map((book) => (
        <Card
          key={book.id}
          className={classes.card}
          elevation={0}
          onClick={() => {
            window.location.hash = `#/audiobook/${book.id}`
          }}
        >
          <CardContent style={{ display: 'flex', padding: '8px 16px' }}>
            <Box className={classes.cover} display="flex" alignItems="center" justifyContent="center">
              {book.coverPath ? (
                <img
                  src={`/api/audiobook/${book.id}/cover`}
                  alt={book.title}
                  className={classes.cover}
                  onError={(e) => {
                    e.target.style.display = 'none'
                    e.target.parentElement.innerHTML = '<span style="font-size:24px">📖</span>'
                  }}
                />
              ) : (
                <MenuBookIcon style={{ fontSize: 24, opacity: 0.5 }} />
              )}
            </Box>
            <Box ml={1.5} flex={1} overflow="hidden">
              <Typography className={classes.title} noWrap>
                {book.title}
              </Typography>
              <Typography className={classes.sub} noWrap>
                {book.author && `${book.author}`}
                {book.narrator && ` · 🎙️ ${book.narrator}`}
              </Typography>
              <Box mt={0.5} display="flex" alignItems="center">
                {book.genre && (
                  <Chip label={book.genre} size="small" className={classes.genre} />
                )}
                <Typography className={classes.sub}>
                  {book.chapterCount} 章
                </Typography>
              </Box>
            </Box>
          </CardContent>
        </Card>
      ))}
    </Box>
  )
}

export default AudiobookList
