import React, { useState, useEffect } from 'react'
import {
  List,
  SimpleList,
  useDataProvider,
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
  useMediaQuery,
  IconButton,
} from '@material-ui/core'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'

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
  const [audiobooks, setAudiobooks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    const fetchAudiobooks = async () => {
      try {
        const token = localStorage.getItem('token')
        const response = await fetch('/api/audiobook', {
          headers: {
            'X-ND-Authorization': `Bearer ${token}`,
          },
        })
        if (!response.ok) {
          throw new Error('Failed to fetch')
        }
        const data = await response.json()
        setAudiobooks(data.data || [])
      } catch (err) {
        setError(err.message)
      } finally {
        setLoading(false)
      }
    }
    fetchAudiobooks()
  }, [])

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
          暂无有声书
        </Typography>
      </Box>
    )
  }

  return (
    <Box p={1}>
      <Typography variant="h6" style={{ padding: '8px 16px', fontWeight: 600 }}>
        📖 有声书 ({audiobooks.length})
      </Typography>
      {audiobooks.map((book) => (
        <Card
          key={book.id}
          className={classes.card}
          elevation={0}
          onClick={() => {
            // Navigate to audiobook detail - for now just show info
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
