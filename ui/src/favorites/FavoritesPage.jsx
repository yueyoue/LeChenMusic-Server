import React, { useState, useEffect, useCallback } from 'react'
import { useDispatch } from 'react-redux'
import {
  Typography,
  Box,
  makeStyles,
  Tabs,
  Tab,
  Card,
  CardContent,
  CircularProgress,
} from '@material-ui/core'
import FavoriteIcon from '@material-ui/icons/Favorite'
import AlbumIcon from '@material-ui/icons/Album'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import { useDataProvider, useNotify } from 'react-admin'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'
import { setTrack } from '../actions'

const useStyles = makeStyles((theme) => ({
  root: { padding: 12 },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: 12,
    padding: '8px 16px',
    marginBottom: 8,
  },
  headerIcon: {
    fontSize: 28,
    color: theme.palette.error.main,
  },
  tabs: {
    marginBottom: 16,
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  tab: {
    textTransform: 'none',
    fontWeight: 600,
    fontSize: 14,
  },
  songList: {
    display: 'flex',
    flexDirection: 'column',
    gap: 4,
  },
  songRow: {
    display: 'flex',
    alignItems: 'center',
    padding: '8px 12px',
    borderRadius: 8,
    cursor: 'pointer',
    transition: 'background 0.15s',
    '&:hover': {
      backgroundColor: theme.palette.action.hover,
    },
  },
  songInfo: {
    flex: 1,
    marginLeft: 12,
    minWidth: 0,
  },
  songTitle: {
    fontSize: 14,
    fontWeight: 500,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  songArtist: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  songDuration: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    marginLeft: 16,
    flexShrink: 0,
  },
  empty: {
    textAlign: 'center',
    padding: 60,
    color: theme.palette.text.secondary,
  },
  audiobookGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(140px, 1fr))',
    gap: 12,
    padding: '0 4px',
  },
  audiobookCard: {
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
  audiobookContent: {
    padding: '8px 10px !important',
    '&:last-child': { paddingBottom: '8px !important' },
  },
  audiobookTitle: {
    fontWeight: 600,
    fontSize: 13,
    lineHeight: 1.3,
    display: '-webkit-box',
    WebkitLineClamp: 2,
    WebkitBoxOrient: 'vertical',
    overflow: 'hidden',
  },
  audiobookSub: {
    fontSize: 11,
    color: theme.palette.text.secondary,
    marginTop: 2,
  },
}))

const formatDuration = (seconds) => {
  if (!seconds) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

const FavoritesPage = () => {
  const classes = useStyles()
  const notify = useNotify()
  const dispatch = useDispatch()
  const dataProvider = useDataProvider()
  const [tab, setTab] = useState(0)
  const [loading, setLoading] = useState(true)
  const [starredAlbums, setStarredAlbums] = useState([])
  const [starredSongs, setStarredSongs] = useState([])
  const [starredAudiobooks, setStarredAudiobooks] = useState([])

  useEffect(() => {
    const fetchFavorites = async () => {
      setLoading(true)
      try {
        // Use react-admin data provider for albums and songs
        const [albumsResult, songsResult] = await Promise.all([
          dataProvider.getList('album', {
            pagination: { page: 1, perPage: 100 },
            sort: { field: 'starred_at', order: 'DESC' },
            filter: { starred: true },
          }).catch(e => {
            console.error('Failed to fetch starred albums:', e)
            return { data: [] }
          }),
          dataProvider.getList('song', {
            pagination: { page: 1, perPage: 100 },
            sort: { field: 'starred_at', order: 'DESC' },
            filter: { starred: true },
          }).catch(e => {
            console.error('Failed to fetch starred songs:', e)
            return { data: [] }
          }),
        ])

        setStarredAlbums(albumsResult.data || [])
        setStarredSongs(songsResult.data || [])

        // Fetch starred audiobooks via custom API
        try {
          const token = localStorage.getItem('token')
          const res = await httpClient(`${REST_URL}/audiobook/starred`)
          const data = res.json
          setStarredAudiobooks(data?.data || [])
        } catch (e) {
          console.error('Failed to fetch starred audiobooks:', e)
        }
      } catch (err) {
        console.error('Failed to fetch favorites:', err)
        notify('加载收藏失败', 'warning')
      } finally {
        setLoading(false)
      }
    }
    fetchFavorites()
  }, [dataProvider, notify])

  if (loading) {
    return (
      <Box p={4} textAlign="center">
        <CircularProgress />
      </Box>
    )
  }

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <FavoriteIcon className={classes.headerIcon} />
        <Typography variant="h6" style={{ fontWeight: 700 }}>
          我的收藏
        </Typography>
      </Box>

      <Tabs
        value={tab}
        onChange={(_, v) => setTab(v)}
        className={classes.tabs}
        indicatorColor="primary"
        textColor="primary"
      >
        <Tab
          label={`专辑 (${starredAlbums.length})`}
          icon={<AlbumIcon />}
          className={classes.tab}
        />
        <Tab
          label={`歌曲 (${starredSongs.length})`}
          icon={<MusicNoteIcon />}
          className={classes.tab}
        />
        <Tab
          label={`有声书 (${starredAudiobooks.length})`}
          icon={<MenuBookIcon />}
          className={classes.tab}
        />
      </Tabs>

      {/* Albums Tab */}
      {tab === 0 && (
        <Box>
          {starredAlbums.length === 0 ? (
            <Box className={classes.empty}>
              <AlbumIcon style={{ fontSize: 48, opacity: 0.3 }} />
              <Typography style={{ marginTop: 8 }}>暂无收藏的专辑</Typography>
              <Typography style={{ marginTop: 4, fontSize: 12, color: 'text.secondary' }}>
                在专辑列表中点击 ❤️ 即可收藏
              </Typography>
            </Box>
          ) : (
            <Box className={classes.audiobookGrid}>
              {starredAlbums.map((album) => (
                <Card
                  key={album.id}
                  className={classes.audiobookCard}
                  elevation={2}
                  onClick={() => { window.location.hash = `#/album/${album.id}/show` }}
                >
                  <Box className={classes.coverWrap}>
                    {album.coverArt ? (
                      <img
                        src={`/rest/getCoverArt?u=${encodeURIComponent(localStorage.getItem('username') || '')}&p=enc:${encodeURIComponent(localStorage.getItem('subsonic-token') || '')}&v=1.8.0&c=LeChenMusic&id=${album.coverArt}`}
                        alt={album.name}
                        className={classes.cover}
                        onError={(e) => { e.target.style.display = 'none' }}
                      />
                    ) : (
                      <Box className={classes.cover} style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)' }}>
                        <AlbumIcon style={{ fontSize: 32, color: 'white', opacity: 0.8 }} />
                      </Box>
                    )}
                  </Box>
                  <CardContent className={classes.audiobookContent}>
                    <Typography className={classes.audiobookTitle}>{album.name}</Typography>
                    <Typography className={classes.audiobookSub}>
                      {album.artist} · {album.songCount || 0}首
                    </Typography>
                  </CardContent>
                </Card>
              ))}
            </Box>
          )}
        </Box>
      )}

      {/* Songs Tab */}
      {tab === 1 && (
        <Box>
          {starredSongs.length === 0 ? (
            <Box className={classes.empty}>
              <MusicNoteIcon style={{ fontSize: 48, opacity: 0.3 }} />
              <Typography style={{ marginTop: 8 }}>暂无收藏的歌曲</Typography>
              <Typography style={{ marginTop: 4, fontSize: 12, color: 'text.secondary' }}>
                在歌曲列表中点击 ❤️ 即可收藏
              </Typography>
            </Box>
          ) : (
            <Box className={classes.songList}>
              {starredSongs.map((song, idx) => (
                <Box
                  key={song.id}
                  className={classes.songRow}
                  onClick={() => { dispatch(setTrack(song)) }}
                >
                  <Typography style={{ width: 32, textAlign: 'center', fontSize: 13, color: 'text.secondary' }}>
                    {idx + 1}
                  </Typography>
                  <Box style={{ width: 40, height: 40, borderRadius: 6, overflow: 'hidden', flexShrink: 0 }}>
                    {song.coverArt ? (
                      <img
                        src={`/rest/getCoverArt?u=${encodeURIComponent(localStorage.getItem('username') || '')}&p=enc:${encodeURIComponent(localStorage.getItem('subsonic-token') || '')}&v=1.8.0&c=LeChenMusic&id=${song.coverArt}`}
                        alt=""
                        style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                        onError={(e) => { e.target.style.display = 'none' }}
                      />
                    ) : (
                      <Box style={{ width: '100%', height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center', background: '#e0e0e0' }}>
                        <MusicNoteIcon style={{ fontSize: 20, opacity: 0.3 }} />
                      </Box>
                    )}
                  </Box>
                  <Box className={classes.songInfo}>
                    <Typography className={classes.songTitle}>{song.title}</Typography>
                    <Typography className={classes.songArtist}>{song.artist}</Typography>
                  </Box>
                  <Typography className={classes.songDuration}>
                    {formatDuration(song.duration)}
                  </Typography>
                </Box>
              ))}
            </Box>
          )}
        </Box>
      )}

      {/* Audiobooks Tab */}
      {tab === 2 && (
        <Box>
          {starredAudiobooks.length === 0 ? (
            <Box className={classes.empty}>
              <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
              <Typography style={{ marginTop: 8 }}>暂无收藏的有声书</Typography>
              <Typography style={{ marginTop: 4, fontSize: 12, color: 'text.secondary' }}>
                在有声书详情页中点击 ❤️ 即可收藏
              </Typography>
            </Box>
          ) : (
            <Box className={classes.audiobookGrid}>
              {starredAudiobooks.map((book) => (
                <Card
                  key={book.id}
                  className={classes.audiobookCard}
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
                  <CardContent className={classes.audiobookContent}>
                    <Typography className={classes.audiobookTitle}>{book.title}</Typography>
                    <Typography className={classes.audiobookSub}>
                      {book.author} · {book.chapterCount}章
                    </Typography>
                  </CardContent>
                </Card>
              ))}
            </Box>
          )}
        </Box>
      )}
    </Box>
  )
}

export default FavoritesPage
