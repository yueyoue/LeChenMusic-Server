import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Avatar,
  TextField, InputAdornment
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import PersonIcon from '@material-ui/icons/Person'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16 },
  search: { marginBottom: 16 },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))',
    gap: 12,
  },
  card: {
    cursor: 'pointer', textAlign: 'center', padding: '20px 12px !important',
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  avatar: {
    width: 64, height: 64, margin: '0 auto 12px',
    backgroundColor: theme.palette.primary.main,
    fontSize: 28,
  },
  name: { fontSize: 14, fontWeight: 600, marginBottom: 4 },
  count: { fontSize: 12, color: theme.palette.text.secondary },
  empty: { textAlign: 'center', padding: 40, color: theme.palette.text.secondary },
}))

const NarratorList = () => {
  const classes = useStyles()
  const [narrators, setNarrators] = useState([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState('')

  useEffect(() => {
    const fetchNarrators = async () => {
      try {
        const token = localStorage.getItem('token')
        const res = await fetch('/api/audiobook/narrators', {
          headers: { 'X-ND-Authorization': `Bearer ${token}` },
        })
        if (res.ok) {
          const data = await res.json()
          setNarrators(data.data || [])
        }
      } catch (err) {
        console.error('Failed to load narrators:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchNarrators()
  }, [])

  const filtered = narrators.filter(n =>
    n.name.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <Box className={classes.root}>
      <Typography variant="h6" style={{ fontWeight: 700, marginBottom: 16 }}>
        🎤 演播者
      </Typography>

      <TextField
        className={classes.search}
        fullWidth
        size="small"
        placeholder="搜索演播者..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon style={{ fontSize: 20, color: 'text.secondary' }} />
            </InputAdornment>
          ),
        }}
        variant="outlined"
      />

      {loading ? (
        <Box p={2} textAlign="center"><Typography>加载中...</Typography></Box>
      ) : filtered.length === 0 ? (
        <Box className={classes.empty}>
          <PersonIcon style={{ fontSize: 48, opacity: 0.3 }} />
          <Typography style={{ marginTop: 8 }}>
            {search ? '未找到匹配的演播者' : '暂无演播者'}
          </Typography>
        </Box>
      ) : (
        <Box className={classes.grid}>
          {filtered.map(narrator => (
            <Card key={narrator.name} className={classes.card} elevation={0}
              onClick={() => window.location.hash = `#/narrator/${encodeURIComponent(narrator.name)}`}>
              <Avatar className={classes.avatar}>
                {narrator.name.charAt(0)}
              </Avatar>
              <Typography className={classes.name}>{narrator.name}</Typography>
              <Typography className={classes.count}>{narrator.count} 部作品</Typography>
            </Card>
          ))}
        </Box>
      )}
    </Box>
  )
}

export default NarratorList
