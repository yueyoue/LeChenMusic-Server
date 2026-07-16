import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Avatar,
  IconButton, Chip, Button, Tooltip
} from '@material-ui/core'
import ArrowBackIcon from '@material-ui/icons/ArrowBack'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import PersonIcon from '@material-ui/icons/Person'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import ArtistAvatarDialog from '../scraper/ArtistAvatarDialog'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16 },
  header: {
    display: 'flex', alignItems: 'center', gap: 20, marginBottom: 24,
    padding: '20px 0',
  },
  avatar: {
    width: 80, height: 80, fontSize: 36,
    backgroundColor: theme.palette.primary.main,
  },
  name: { fontSize: 24, fontWeight: 700, marginBottom: 4 },
  stats: { fontSize: 14, color: theme.palette.text.secondary },
  backBtn: { marginBottom: 8 },
  card: {
    marginBottom: 8, cursor: 'pointer',
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  cover: {
    width: 60, height: 60, borderRadius: 4, objectFit: 'cover',
    backgroundColor: theme.palette.grey[300],
  },
  bookTitle: { fontWeight: 600, fontSize: 14 },
  bookSub: { fontSize: 12, color: theme.palette.text.secondary },
  sectionTitle: {
    fontSize: 16, fontWeight: 600, marginBottom: 12,
    display: 'flex', alignItems: 'center', gap: 8,
  },
  empty: { textAlign: 'center', padding: 40, color: theme.palette.text.secondary },
}))

const NarratorDetail = ({ name, onBack, onPlayBook }) => {
  const classes = useStyles()
  const [works, setWorks] = useState([])
  const [loading, setLoading] = useState(true)
  const [avatarDialogOpen, setAvatarDialogOpen] = useState(false)
  const [avatarUrl, setAvatarUrl] = useState(null)

  useEffect(() => {
    // Check if narrator avatar exists
    const checkAvatar = async () => {
      try {
        const token = localStorage.getItem('token')
        const safeName = name.replace(/\//g, '_').replace(/\\/g, '_')
        const tokenParam = token ? `?token=${token}` : ''
        const res = await fetch(`/api/scrape/image/narrator/${encodeURIComponent(safeName)}${tokenParam}`)
        if (res.ok) {
          setAvatarUrl(`/api/scrape/image/narrator/${encodeURIComponent(safeName)}${tokenParam}`)
        }
      } catch (e) {}
    }
    checkAvatar()
  }, [name])

  useEffect(() => {
    const fetchWorks = async () => {
      try {
        const token = localStorage.getItem('token')
        const res = await fetch(`/api/audiobook/narrator/${encodeURIComponent(name)}`, {
          headers: { 'X-ND-Authorization': `Bearer ${token}` },
        })
        if (res.ok) {
          const data = await res.json()
          setWorks(data.data?.works || [])
        }
      } catch (err) {
        console.error('Failed to load narrator works:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchWorks()
  }, [name])

  if (loading) {
    return <Box p={4} textAlign="center"><Typography>加载中...</Typography></Box>
  }

  return (
    <>
    <Box className={classes.root}>
      <IconButton className={classes.backBtn} onClick={onBack}>
        <ArrowBackIcon />
      </IconButton>

      <Box className={classes.header}>
        {avatarUrl ? (
          <img src={avatarUrl} alt={name} className={classes.avatar}
            style={{ borderRadius: '50%', objectFit: 'cover' }} />
        ) : (
          <Avatar className={classes.avatar}>{name?.charAt(0) || '?'}</Avatar>
        )}
        <Box>
          <Typography className={classes.name}>{name}</Typography>
          <Typography className={classes.stats}>🎙️ 演播者 · {works.length} 部作品</Typography>
          <Tooltip title="搜索匹配演播者头像">
            <Button size="small" variant="outlined" style={{ marginTop: 8 }}
              onClick={() => setAvatarDialogOpen(true)}>
              🔍 匹配头像
            </Button>
          </Tooltip>
        </Box>
      </Box>

      <Typography className={classes.sectionTitle}>
        📚 全部作品 ({works.length})
      </Typography>

      {works.length === 0 ? (
        <Box className={classes.empty}>
          <MenuBookIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 8 }}>暂无作品</Typography>
        </Box>
      ) : (
        works.map(book => (
          <Card key={book.id} className={classes.card} elevation={0}
            onClick={() => window.location.hash = `#/audiobook/${book.id}`}>
            <CardContent style={{ display: 'flex', padding: '8px 16px' }}>
              <Box className={classes.cover} display="flex" alignItems="center" justifyContent="center">
                {book.coverPath ? (
                  <img src={`/api/audiobook/${book.id}/cover`} alt={book.title} className={classes.cover}
                    onError={(e) => { e.target.style.display = 'none' }} />
                ) : (
                  <MenuBookIcon style={{ fontSize: 24, opacity: 0.5 }} />
                )}
              </Box>
              <Box ml={1.5} flex={1} overflow="hidden">
                <Typography className={classes.bookTitle} noWrap>{book.title}</Typography>
                <Typography className={classes.bookSub} noWrap>
                  {book.author && `${book.author} · `}{book.chapterCount} 章
                </Typography>
                {book.genre && <Chip label={book.genre} size="small" style={{ fontSize: 11, marginTop: 4 }} />}
              </Box>
            </CardContent>
          </Card>
        ))
      )}
    </Box>
    <ArtistAvatarDialog
      open={avatarDialogOpen}
      onClose={() => setAvatarDialogOpen(false)}
      artist={{ id: `narrator-${name}`, name: name }}
      searchType="narrator"
      onApply={() => window.location.reload()}
    />
    </>
  )
}

export default NarratorDetail
