import React, { useState, useEffect } from 'react'
import { useTranslate, useRefresh, useDataProvider, useListContext } from 'react-admin'
import {
  GridList, GridListTile, GridListTileBar,
  Typography, Box, Chip, makeStyles, TextField, InputAdornment, Button, useMediaQuery,
} from '@material-ui/core'
import withWidth from '@material-ui/core/withWidth'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import SearchIcon from '@material-ui/icons/Search'
import ScrapeDialog from '../scraper/ScrapeDialog'
import { useLocation } from 'react-router-dom'
import { OverflowTooltip } from '../common'

const useStyles = makeStyles((theme) => ({
  root: { padding: 12 },
  gridListTile: {
    '&:hover $tileBar': {
      opacity: 1,
    },
  },
  tileBar: {
    transition: 'all 150ms ease-out',
    opacity: 0,
    pointerEvents: 'none',
    textAlign: 'left',
    background:
      'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
  },
  tileBarMobile: {
    textAlign: 'left',
    background:
      'linear-gradient(to top, rgba(0,0,0,0.7) 0%,rgba(0,0,0,0.4) 70%,rgba(0,0,0,0) 100%)',
  },
  coverContainer: {
    width: '100%',
    aspectRatio: '1',
    overflow: 'hidden',
    backgroundColor: theme.palette.grey[200],
    borderRadius: 4,
  },
  cover: {
    display: 'inline-block',
    width: '100%',
    height: '100%',
    objectFit: 'cover',
  },
  coverPlaceholder: {
    width: '100%',
    height: '100%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
  },
  albumContainer: {
    cursor: 'pointer',
  },
  albumLink: {
    textDecoration: 'none',
    color: 'inherit',
    display: 'block',
  },
  albumName: {
    fontSize: '14px',
    color: theme.palette.type === 'dark' ? '#eee' : 'black',
    overflow: 'hidden',
    whiteSpace: 'nowrap',
    textOverflow: 'ellipsis',
  },
  albumSubtitle: {
    fontSize: '12px',
    color: theme.palette.type === 'dark' ? '#c5c5c5' : '#696969',
    overflow: 'hidden',
    whiteSpace: 'nowrap',
    textOverflow: 'ellipsis',
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
    display: 'flex',
    alignItems: 'center',
    gap: 12,
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
}))

const getColsForWidth = (width) => {
  if (width === 'xs') return 2
  if (width === 'sm') return 3
  if (width === 'md') return 4
  if (width === 'lg') return 6
  return 9
}

const AudiobookCover = ({ book }) => {
  const classes = useStyles()
  const token = localStorage.getItem('token')
  const url = `/api/audiobook/${book.id}/cover?token=${token || ''}`
  const [imgError, setImgError] = useState(false)

  if (imgError || !book.coverPath) {
    return (
      <div className={classes.coverPlaceholder}>
        <MenuBookIcon style={{ fontSize: 32, opacity: 0.5, color: '#fff' }} />
      </div>
    )
  }

  return (
    <img
      src={url}
      alt={book.title}
      className={classes.cover}
      onError={() => setImgError(true)}
    />
  )
}

const AudiobookGridTile = ({ book }) => {
  const classes = useStyles()
  const isDesktop = useMediaQuery((theme) => theme.breakpoints.up('md'), { noSsr: true })

  return (
    <div className={classes.albumContainer}>
      <div onClick={() => { window.location.hash = `#/audiobook/${book.id}` }}>
        <div className={classes.coverContainer}>
          <AudiobookCover book={book} />
        </div>
        <GridListTileBar
          className={isDesktop ? classes.tileBar : classes.tileBarMobile}
        />
      </div>
      <div onClick={() => { window.location.hash = `#/audiobook/${book.id}` }}>
        <OverflowTooltip title={book.title}>
          <Typography className={classes.albumName}>{book.title}</Typography>
        </OverflowTooltip>
        <Typography className={classes.albumSubtitle}>
          {book.narrator || book.author || ''}
        </Typography>
      </div>
    </div>
  )
}

const AudiobookList = ({ width }) => {
  const classes = useStyles()
  const translate = useTranslate()
  const location = useLocation()
  const [audiobooks, setAudiobooks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState(null)
  const [scrapeOpen, setScrapeOpen] = useState(false)
  const [rescanning, setRescanning] = useState(false)

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
    <>
      <Box className={classes.root}>
        <Typography className={classes.header}>
          {getTitle()} ({displayBooks.length})
          <Button size="small" variant="outlined" onClick={() => setScrapeOpen(true)}>
            🔍 批量刮削
          </Button>
          <Button size="small" variant="outlined" disabled={rescanning}
            onClick={async () => {
              if (!confirm('将为所有没有章节的有声书重新扫描章节，确定继续？')) return
              setRescanning(true)
              try {
                const token = localStorage.getItem('token')
                const res = await fetch('/api/audiobook/rescan-all', {
                  method: 'POST',
                  headers: { 'X-ND-Authorization': `Bearer ${token}` },
                })
                if (res.ok) {
                  const data = await res.json()
                  const d = data.data
                  alert(`扫描完成：${d.rescanned} 本已扫描，${d.skipped} 本跳过，${d.failed} 本失败`)
                  window.location.reload()
                } else {
                  alert('扫描失败: ' + res.statusText)
                }
              } catch (e) {
                alert('扫描失败: ' + e.message)
              } finally {
                setRescanning(false)
              }
            }}>
            {rescanning ? '⏳ 扫描中...' : '🔄 扫描缺失章节'}
          </Button>
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
          <GridList
            component="div"
            cellHeight="auto"
            cols={getColsForWidth(width)}
            spacing={16}
          >
            {displayBooks.map((book) => (
              <GridListTile className={classes.gridListTile} key={book.id}>
                <AudiobookGridTile book={book} />
              </GridListTile>
            ))}
          </GridList>
        )}
      </Box>
      <ScrapeDialog open={scrapeOpen} onClose={() => setScrapeOpen(false)} />
    </>
  )
}

export default withWidth()(AudiobookList)
