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
  TextField,
  InputAdornment,
} from '@material-ui/core'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import SearchIcon from '@material-ui/icons/Search'
import PersonIcon from '@material-ui/icons/Person'
import { useLocation } from 'react-router-dom'

const useStyles = makeStyles((theme) => ({
  root: { padding: 12 },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
    gap: 12,
    padding: '0 4px',
  },
  card: {
    cursor: 'pointer',
    borderRadius: 12,
    overflow: 'hidden',
    transition: 'transform 0.2s, box-shadow 0.2s',
    '&:hover': {
      transform: 'translateY(-2px)',
      boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
    },
  },
  coverWrap: {
    position: 'relative',
    width: '100%',
    paddingTop: '100%', /* Square like music albums */
    backgroundColor: theme.palette.grey[200],
    borderRadius: '12px 12px 0 0',
    overflow: 'hidden',
  },
  cover: {
    position: 'absolute',
    top: 0,
    left: 0,
    width: '100%',
    height: '100%',
    objectFit: 'cover',
  },
  coverPlaceholder: {
    position: 'absolute',
    top: 0,
    left: 0,
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
  },
  cardContent: {
    padding: '8px 10px !important',
    '&:last-child': { paddingBottom: '8px !important' },
  },
  title: {
    fontWeight: 600,
    fontSize: 13,
    lineHeight: 1.3,
    display: '-webkit-box',
    WebkitLineClamp: 2,
    WebkitBoxOrient: 'vertical',
    overflow: 'hidden',
    marginBottom: 4,
  },
  sub: {
    fontSize: 11,
    color: theme.palette.text.secondary,
    display: 'flex',
    alignItems: 'center',
    gap: 4,
  },
  narratorBadge: {
    fontSize: 10,
    padding: '1px 6px',
    borderRadius: 4,
    backgroundColor: theme.palette.primary.light + '20',
    color: theme.palette.primary.main,
    fontWeight: 500,
  },
  genre: {
    fontSize: 10,
    marginRight: 4,
    height: 20,
  },
  empty: {
    textAlign: 'center',
    padding: 60,
    color: theme.palette.text.secondary,
  },
  search: { marginBottom: 12 },
  header: {
    padding: '8px 16px',
    fontWeight: 600,
    fontSize: 16,
  },
}))

const AudiobookList = () => {
  const classes = useStyles()
  const translate = useTranslate()
  const location = useLocation()
  const [audiobooks, setAudiobooks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState(null)

  const params = new URLSearchParams(location.search)
  const genreFilter = params.get('genre')
  const isStarred = location.pathname.includes('/starred')

  useEffect(() => {
    const fetchAudiobooks = async () => {
      try {
        const token = localStorage.getItem('token')
        if (!token) {
          setError('请先登录')
          setLoading(false)
          return
        }
        let url = '/api/audiobook'
        if (isStarred) url = '/api/audiobook/starred'
        const response = await fetch(url, {
          headers: { 'X-ND-Authorization': `Bearer ${token}` },
        })
        if (!response.ok) {
          const errText = await response.text().catch(() => '')
          throw new Error(`服务器错误 (${response.status}): ${errText || response.statusText}`)
        }
        const data = await response.json()
        let books = data.data || []
        if (genreFilter) books = books.filter((b) => b.genre === genreFilter)
        setAudiobooks(books)
        setError(null)
      } catch (err) {
        console.error('Failed to fetch audiobooks:', err)
        setError(err.message || '加载失败，请检查网络连接')
      } finally {
        setLoading(false)
      }
    }
    fetchAudiobooks()
  }, [genreFilter, isStarred])

  useEffect(() => {
    if (!searchQuery.trim()) { setSearchResults(null); return }
    const timer = setTimeout(async () => {
      try {
        const token = localStorage.getItem('token')
        const res = await fetch(`/api/audiobook/search?q=${encodeURIComponent(searchQuery)}`, {
          headers: { 'X-ND-Authorization': `Bearer ${token}` },
        })
        if (res.ok) {
          const data = await res.json()
          setSearchResults(data.data || [])
        }
      } catch (err) { console.error('Search failed:', err) }
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  const getTitle = () => {
    if (isStarred) return '⭐ 收藏的有声书'
    if (genreFilter) return `📖 ${genreFilter}`
    return '📖 全部有声书'
  }

  const displayBooks = searchResults !== null ? searchResults : audiobooks

  if (loading) {
    return <Box p={2} textAlign="center"><Typography>{translate('ra.loading')}...</Typography></Box>
  }
  if (error) {
    return <Box p={2} textAlign="center"><Typography color="error">{error}</Typography></Box>
  }

  return (
    <Box className={classes.root}>
      <Typography className={classes.header}>
        {getTitle()} ({displayBooks.length})
      </Typography>

      <Box px={1} mb={1}>
        <TextField
          className={classes.search}
          fullWidth
          size="small"
          placeholder="搜索有声书（书名、作者、演播者）..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon style={{ fontSize: 20, color: 'text.secondary' }} />
              </InputAdornment>
            ),
          }}
          variant="outlined"
        />
      </Box>

      {displayBooks.length === 0 ? (
        <Box className={classes.empty}>
          <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 8 }}>
            {searchQuery ? '未找到匹配的有声书' : genreFilter ? `暂无${genreFilter}` : '暂无有声书'}
          </Typography>
        </Box>
      ) : (
        <Box className={classes.grid}>
          {displayBooks.map((book) => (
            <Card
              key={book.id}
              className={classes.card}
              elevation={2}
              onClick={() => { window.location.hash = `#/audiobook/${book.id}` }}
            >
              <Box className={classes.coverWrap}>
                <img
                  src={`/api/audiobook/${book.id}/cover?token=${localStorage.getItem('token') || ''}`}
                  alt={book.title}
                  className={classes.cover}
                  onError={(e) => {
                    e.target.style.display = 'none'
                    e.target.parentElement.innerHTML = '<span style="font-size:32px">📖</span>'
                  }}
                />
              </Box>
              <CardContent className={classes.cardContent}>
                <Typography className={classes.title}>{book.title}</Typography>
                <Box className={classes.sub}>
                  {book.author && <span>{book.author}</span>}
                  {book.narrator && (
                    <span className={classes.narratorBadge}>🎙️ {book.narrator}</span>
                  )}
                </Box>
                <Box mt={0.5} display="flex" alignItems="center" gap={0.5}>
                  {book.genre && <Chip label={book.genre} size="small" className={classes.genre} />}
                  <Typography className={classes.sub}>{book.chapterCount}章</Typography>
                </Box>
              </CardContent>
            </Card>
          ))}
        </Box>
      )}
    </Box>
  )
}

export default AudiobookList

