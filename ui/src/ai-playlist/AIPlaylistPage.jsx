import React, { useState, useEffect } from 'react'
import {
  Typography, Box, Card, CardContent, makeStyles, Button, TextField,
  Chip, CircularProgress, Dialog, DialogTitle, DialogContent, DialogActions,
  Checkbox, FormControlLabel, InputAdornment, IconButton, Tooltip, Divider,
  Select, MenuItem, FormControl, InputLabel
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import PlaylistAddIcon from '@material-ui/icons/PlaylistAdd'
import LinkIcon from '@material-ui/icons/Link'
import ImageIcon from '@material-ui/icons/Image'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import CancelIcon from '@material-ui/icons/Cancel'

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 900, margin: '0 auto' },
  header: {
    display: 'flex', alignItems: 'center', gap: 12, marginBottom: 24,
    '& h5': { fontWeight: 700 },
  },
  searchCard: { marginBottom: 16, padding: '16px !important' },
  searchRow: { display: 'flex', gap: 12, alignItems: 'flex-start' },
  searchInput: { flex: 1 },
  sourceChips: { display: 'flex', gap: 8, marginTop: 12, flexWrap: 'wrap' },
  resultsCard: { marginBottom: 16 },
  matchStats: {
    display: 'flex', gap: 16, padding: '12px 16px',
    backgroundColor: theme.palette.background.default,
    borderRadius: 8, marginBottom: 16,
  },
  statItem: { textAlign: 'center' },
  statValue: { fontSize: 24, fontWeight: 700, color: theme.palette.primary.main },
  statLabel: { fontSize: 12, color: theme.palette.text.secondary },
  songList: { maxHeight: 400, overflow: 'auto' },
  songRow: {
    display: 'flex', alignItems: 'center', padding: '8px 12px',
    borderRadius: 8, cursor: 'pointer', marginBottom: 2,
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  songChecked: { backgroundColor: theme.palette.action.selected },
  songInfo: { flex: 1, marginLeft: 12 },
  songTitle: { fontSize: 14, fontWeight: 500 },
  songArtist: { fontSize: 12, color: theme.palette.text.secondary },
  songSource: { marginLeft: 8 },
  unmatchedSection: { marginTop: 16 },
  unmatchedTitle: { fontSize: 14, fontWeight: 600, marginBottom: 8, color: theme.palette.text.secondary },
  unmatchedSong: { fontSize: 12, color: theme.palette.text.secondary, padding: '4px 0' },
  createSection: { marginTop: 16, padding: 16, backgroundColor: theme.palette.background.default, borderRadius: 8 },
  createRow: { display: 'flex', gap: 12, alignItems: 'center', marginTop: 12 },
  coverPreview: { width: 120, height: 120, borderRadius: 8, objectFit: 'cover', border: `1px solid ${theme.palette.divider}` },
  themeSelect: { minWidth: 120 },
  empty: { textAlign: 'center', padding: 60, color: theme.palette.text.secondary },
  urlSection: { marginTop: 8 },
  divider: { margin: '16px 0' },
}))

const AIPlaylistPage = () => {
  const classes = useStyles()
  const [query, setQuery] = useState('')
  const [urlInput, setUrlInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(null)
  const [selectedSongs, setSelectedSongs] = useState(new Set())
  const [playlistName, setPlaylistName] = useState('')
  const [creating, setCreating] = useState(false)
  const [created, setCreated] = useState(null)
  const [coverTheme, setCoverTheme] = useState('')
  const [coverEnabled, setCoverEnabled] = useState(true)
  const [coverPreviewUrl, setCoverPreviewUrl] = useState(null)
  const [importedCoverUrl, setImportedCoverUrl] = useState('')
  const [themes, setThemes] = useState([])
  const [sources, setSources] = useState(['酷我', '网易云', 'QQ音乐', '酷狗', '汽水音乐'])

  const getToken = () => localStorage.getItem('token')
  const getHeaders = () => ({ 'X-ND-Authorization': `Bearer ${getToken()}`, 'Content-Type': 'application/json' })

  // Load themes
  useEffect(() => {
    fetch('/api/ai-playlist/cover/themes', { headers: getHeaders() })
      .then(r => r.json())
      .then(d => setThemes(d.themes || []))
      .catch(() => {})
  }, [])

  const handleSearch = async () => {
    if (!query.trim()) return
    setLoading(true)
    setResults(null)
    setCreated(null)
    try {
      const res = await fetch('/api/ai-playlist/match', {
        method: 'POST',
        headers: getHeaders(),
        body: JSON.stringify({ query, sources }),
      })
      if (res.ok) {
        const data = await res.json()
        setResults(data)
        // Auto-select all matched songs
        setSelectedSongs(new Set(data.matched.map((_, i) => i)))
        setPlaylistName(query)
      }
    } catch (err) {
      console.error('Search failed:', err)
    } finally {
      setLoading(false)
    }
  }

  const handleImportURL = async () => {
    if (!urlInput.trim()) return
    setLoading(true)
    setResults(null)
    setCreated(null)
    setImportedCoverUrl('')
    try {
      const res = await fetch('/api/ai-playlist/from-url', {
        method: 'POST',
        headers: getHeaders(),
        body: JSON.stringify({ url: urlInput }),
      })
      if (res.ok) {
        const data = await res.json()
        setResults(data)
        setSelectedSongs(new Set(data.matched.map((_, i) => i)))
        setPlaylistName(data.playlistName || '导入歌单')
        if (data.coverURL) {
          setImportedCoverUrl(data.coverURL)
        }
      }
    } catch (err) {
      console.error('Import failed:', err)
    } finally {
      setLoading(false)
    }
  }

  const toggleSong = (index) => {
    const next = new Set(selectedSongs)
    if (next.has(index)) next.delete(index)
    else next.add(index)
    setSelectedSongs(next)
  }

  const handleCreate = async () => {
    if (!playlistName.trim() || selectedSongs.size === 0) return
    setCreating(true)
    try {
      const songIds = results.matched
        .filter((_, i) => selectedSongs.has(i))
        .map(s => s.id)
      const res = await fetch('/api/ai-playlist/create', {
        method: 'POST',
        headers: getHeaders(),
        body: JSON.stringify({
          name: playlistName,
          songIds,
          coverTheme,
          coverEnabled: importedCoverUrl ? false : coverEnabled,
          coverURL: importedCoverUrl || '',
        }),
      })
      if (res.ok) {
        const data = await res.json()
        setCreated(data)
      }
    } catch (err) {
      console.error('Create failed:', err)
    } finally {
      setCreating(false)
    }
  }

  const toggleSource = (source) => {
    setSources(prev =>
      prev.includes(source)
        ? prev.filter(s => s !== source)
        : [...prev, source]
    )
  }

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <MusicNoteIcon style={{ fontSize: 28, color: 'primary.main' }} />
        <Typography variant="h5">AI 智能歌单</Typography>
      </Box>

      {/* Search Section */}
      <Card className={classes.searchCard} elevation={1}>
        <Typography style={{ fontWeight: 600, marginBottom: 8 }}>🔍 搜索主题创建</Typography>
        <Box className={classes.searchRow}>
          <TextField
            className={classes.searchInput}
            size="small"
            variant="outlined"
            placeholder="输入歌单主题，如：民谣歌单、80后经典、周杰伦精选、深夜emo..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
            InputProps={{
              startAdornment: <InputAdornment position="start"><SearchIcon /></InputAdornment>,
            }}
          />
          <Button
            variant="contained"
            color="primary"
            onClick={handleSearch}
            disabled={loading || !query.trim()}
            style={{ borderRadius: 8, textTransform: 'none', fontWeight: 600 }}
          >
            {loading ? <CircularProgress size={20} /> : 'AI 搜索匹配'}
          </Button>
        </Box>
        <Box className={classes.sourceChips}>
          {['酷我', '网易云', 'QQ音乐', '酷狗', '汽水音乐'].map(src => (
            <Chip
              key={src}
              label={src}
              size="small"
              color={sources.includes(src) ? 'primary' : 'default'}
              variant={sources.includes(src) ? 'default' : 'outlined'}
              onClick={() => toggleSource(src)}
            />
          ))}
        </Box>
      </Card>

      {/* URL Import Section */}
      <Card className={classes.searchCard} elevation={1}>
        <Typography style={{ fontWeight: 600, marginBottom: 8 }}>🔗 歌单链接导入</Typography>
        <Box className={classes.searchRow}>
          <TextField
            className={classes.searchInput}
            size="small"
            variant="outlined"
            placeholder="粘贴网易云/QQ音乐/酷我/酷狗/汽水音乐歌单链接..."
            value={urlInput}
            onChange={(e) => setUrlInput(e.target.value)}
            onKeyPress={(e) => e.key === 'Enter' && handleImportURL()}
            InputProps={{
              startAdornment: <InputAdornment position="start"><LinkIcon /></InputAdornment>,
            }}
          />
          <Button
            variant="outlined"
            color="primary"
            onClick={handleImportURL}
            disabled={loading || !urlInput.trim()}
            style={{ borderRadius: 8, textTransform: 'none', fontWeight: 600 }}
          >
            导入歌单
          </Button>
        </Box>
      </Card>

      {/* TXT File Import Section */}
      <Card className={classes.searchCard} elevation={1}>
        <Typography style={{ fontWeight: 600, marginBottom: 8 }}>📄 TXT文件导入</Typography>
        <Box className={classes.searchRow}>
          <input
            type="file"
            accept=".txt"
            id="txt-file-input"
            style={{ display: 'none' }}
            onChange={async (e) => {
              const file = e.target.files?.[0]
              if (!file) return
              setLoading(true)
              setResults(null)
              setCreated(null)
              setImportedCoverUrl('')
              try {
                const formData = new FormData()
                formData.append('file', file)
                const res = await fetch('/api/ai-playlist/import-txt', {
                  method: 'POST',
                  headers: { 'X-ND-Authorization': `Bearer ${getToken()}` },
                  body: formData,
                })
                if (res.ok) {
                  const data = await res.json()
                  setResults(data)
                  setSelectedSongs(new Set(data.matched.map((_, i) => i)))
                  setPlaylistName(data.playlistName || 'TXT导入歌单')
                } else {
                  const errText = await res.text()
                  alert('导入失败: ' + errText)
                }
              } catch (err) {
                alert('导入失败: ' + err.message)
              } finally {
                setLoading(false)
                e.target.value = ''
              }
            }}
          />
          <Button
            variant="outlined"
            component="span"
            style={{ borderRadius: 8, textTransform: 'none', fontWeight: 600 }}
          >
            选择TXT文件
          </Button>
          <Typography style={{ fontSize: 12, color: 'text.secondary', alignSelf: 'center' }}>
            支持格式：每行一首，如 "歌名 - 歌手" 或 "歌名"
          </Typography>
        </Box>
      </Card>



      {/* Loading */}
      {loading && (
        <Box textAlign="center" py={4}>
          <CircularProgress />
          <Typography style={{ marginTop: 8, color: 'text.secondary' }}>正在搜索并匹配曲库...</Typography>
        </Box>
      )}

      {/* Results */}
      {results && !loading && (
        <>
          {/* Stats */}
          <Box className={classes.matchStats}>
            <Box className={classes.statItem}>
              <Typography className={classes.statValue}>{results.searchTotal || 0}</Typography>
              <Typography className={classes.statLabel}>搜索结果</Typography>
            </Box>
            <Box className={classes.statItem}>
              <Typography className={classes.statValue} style={{ color: '#2ed573' }}>{results.matchedCount || 0}</Typography>
              <Typography className={classes.statLabel}>已匹配</Typography>
            </Box>
            <Box className={classes.statItem}>
              <Typography className={classes.statValue} style={{ color: '#ff4757' }}>{results.unmatchedCount || 0}</Typography>
              <Typography className={classes.statLabel}>未找到</Typography>
            </Box>
            {results.sourceStats && Object.entries(results.sourceStats).map(([name, count]) => (
              <Box key={name} className={classes.statItem}>
                <Typography className={classes.statValue} style={{ fontSize: 16 }}>{count}</Typography>
                <Typography className={classes.statLabel}>{name}</Typography>
              </Box>
            ))}
          </Box>

          {/* Matched Songs */}
          <Card className={classes.resultsCard} elevation={1}>
            <CardContent>
              <Typography style={{ fontWeight: 600, marginBottom: 8 }}>
                ✅ 已匹配歌曲 ({results.matchedCount})
                <Typography component="span" style={{ fontSize: 12, color: 'text.secondary', marginLeft: 8 }}>
                  已选 {selectedSongs.size} 首
                </Typography>
              </Typography>
              <Box className={classes.songList}>
                {results.matched?.map((song, idx) => (
                  <Box
                    key={idx}
                    className={`${classes.songRow} ${selectedSongs.has(idx) ? classes.songChecked : ''}`}
                    onClick={() => toggleSong(idx)}
                  >
                    <Checkbox
                      checked={selectedSongs.has(idx)}
                      size="small"
                      color="primary"
                    />
                    <Box className={classes.songInfo}>
                      <Typography className={classes.songTitle}>{song.title}</Typography>
                      <Typography className={classes.songArtist}>{song.artist} {song.album ? `· ${song.album}` : ''}</Typography>
                    </Box>
                    <Chip
                      label={song.source}
                      size="small"
                      variant="outlined"
                      className={classes.songSource}
                      style={{ fontSize: 10 }}
                    />
                  </Box>
                ))}
              </Box>
            </CardContent>
          </Card>

          {/* Unmatched Songs */}
          {results.unmatched?.length > 0 && (
            <Card className={classes.resultsCard} elevation={1}>
              <CardContent>
                <Typography className={classes.unmatchedTitle}>
                  ❌ 未找到的歌曲 ({results.unmatchedCount})
                </Typography>
                <Box>
                  {results.unmatched.map((song, idx) => (
                    <Typography key={idx} className={classes.unmatchedSong}>
                      {song.title} - {song.artist} ({song.source})
                    </Typography>
                  ))}
                </Box>
              </CardContent>
            </Card>
          )}

          {/* Create Section */}
          {selectedSongs.size > 0 && (
            <Box className={classes.createSection}>
              <Typography style={{ fontWeight: 600 }}>📋 创建歌单</Typography>
              <Box className={classes.createRow}>
                {importedCoverUrl && (
                  <Box style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4 }}>
                    <img src={importedCoverUrl} alt="封面" className={classes.coverPreview} />
                    <Typography style={{ fontSize: 11, color: 'text.secondary' }}>原歌单封面</Typography>
                  </Box>
                )}
                <TextField
                  size="small"
                  variant="outlined"
                  label="歌单名称"
                  value={playlistName}
                  onChange={(e) => setPlaylistName(e.target.value)}
                  style={{ flex: 1 }}
                />
                {!importedCoverUrl && (<>
                <FormControl variant="outlined" size="small" className={classes.themeSelect}>
                  <InputLabel>封面风格</InputLabel>
                  <Select
                    value={coverTheme}
                    onChange={(e) => setCoverTheme(e.target.value)}
                    label="封面风格"
                  >
                    <MenuItem value="">自动匹配</MenuItem>
                    {themes.map(t => <MenuItem key={t} value={t}>{t}</MenuItem>)}
                  </Select>
                </FormControl>
                <FormControlLabel
                  control={<Checkbox checked={coverEnabled} onChange={(e) => setCoverEnabled(e.target.checked)} color="primary" />}
                  label="生成封面"
                />
                </>)}
                <Button
                  variant="contained"
                  color="primary"
                  startIcon={creating ? <CircularProgress size={18} /> : <PlaylistAddIcon />}
                  onClick={handleCreate}
                  disabled={creating || !playlistName.trim()}
                  style={{ borderRadius: 8, textTransform: 'none', fontWeight: 600 }}
                >
                  {creating ? '创建中...' : `创建歌单 (${selectedSongs.size}首)`}
                </Button>
              </Box>
            </Box>
          )}

          {/* Created Success */}
          {created && (
            <Card style={{ marginTop: 16, background: 'rgba(46,213,115,0.1)', border: '1px solid rgba(46,213,115,0.3)' }} elevation={0}>
              <CardContent style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <CheckCircleIcon style={{ color: '#2ed573', fontSize: 32 }} />
                <Box>
                  <Typography style={{ fontWeight: 600, color: '#2ed573' }}>歌单创建成功！</Typography>
                  <Typography style={{ fontSize: 13, color: 'text.secondary' }}>
                    {created.name} · {created.songCount} 首歌曲
                  </Typography>
                </Box>
              </CardContent>
            </Card>
          )}
        </>
      )}

      {/* Empty State */}
      {!results && !loading && (
        <Box className={classes.empty}>
          <MusicNoteIcon style={{ fontSize: 64, opacity: 0.2 }} />
          <Typography style={{ marginTop: 16, fontSize: 16 }}>输入歌单主题、粘贴歌单链接或上传TXT文件开始</Typography>
          <Typography style={{ marginTop: 8, fontSize: 13, color: 'text.secondary' }}>
            支持酷我、网易云、QQ音乐、酷狗、汽水音乐平台搜索 | 支持TXT文件导入
          </Typography>
        </Box>
      )}

      {/* 使用教程 */}
      <Card style={{ marginTop: 24, padding: '16px 20px' }} elevation={1}>
        <Typography variant="h6" gutterBottom style={{ fontWeight: 700, fontSize: 16 }}>
          📖 各平台歌单采集教程
        </Typography>
        <Box style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16, marginTop: 12 }}>
          {/* 网易云音乐 */}
          <Box style={{ padding: 12, border: '1px solid #e0e0e0', borderRadius: 8 }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#e94560' }}>
              🎵 网易云音乐
            </Typography>
            <Typography style={{ fontSize: 12, color: 'text.secondary', lineHeight: 1.8 }}>
              1. 打开网易云音乐APP或网页版<br/>
              2. 进入要采集的歌单页面<br/>
              3. 点击「分享」→「复制链接」<br/>
              4. 链接格式：<code>music.163.com/playlist?id=数字</code>
            </Typography>
          </Box>
          {/* QQ音乐 */}
          <Box style={{ padding: 12, border: '1px solid #e0e0e0', borderRadius: 8 }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#2ed573' }}>
              🎵 QQ音乐
            </Typography>
            <Typography style={{ fontSize: 12, color: 'text.secondary', lineHeight: 1.8 }}>
              1. 打开QQ音乐APP或网页版<br/>
              2. 进入歌单页面，点击「分享」<br/>
              3. 选择「复制链接」<br/>
              4. 链接格式：<code>y.qq.com/n/ryqq/playlist/数字</code>
            </Typography>
          </Box>
          {/* 酷我音乐 */}
          <Box style={{ padding: 12, border: '1px solid #e0e0e0', borderRadius: 8 }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#f39c12' }}>
              🎵 酷我音乐
            </Typography>
            <Typography style={{ fontSize: 12, color: 'text.secondary', lineHeight: 1.8 }}>
              1. 打开酷我音乐APP或网页版<br/>
              2. 进入歌单页面<br/>
              3. 点击「分享」→「复制链接」<br/>
              4. 链接格式：<code>kuwo.cn/playlist_detail/数字</code>
            </Typography>
          </Box>
          {/* 酷狗音乐 */}
          <Box style={{ padding: 12, border: '1px solid #e0e0e0', borderRadius: 8 }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#5352ed' }}>
              🎵 酷狗音乐
            </Typography>
            <Typography style={{ fontSize: 12, color: 'text.secondary', lineHeight: 1.8 }}>
              1. 打开酷狗音乐APP或网页版<br/>
              2. 进入歌单页面<br/>
              3. 点击「分享」→「复制链接」<br/>
              4. 链接格式：<code>kugou.com/special/数字</code>
            </Typography>
          </Box>
          {/* 汽水音乐 */}
          <Box style={{ padding: 12, border: '1px solid #e0e0e0', borderRadius: 8 }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#00b894' }}>
              🎵 汽水音乐
            </Typography>
            <Typography style={{ fontSize: 12, color: 'text.secondary', lineHeight: 1.8 }}>
              1. 打开汽水音乐APP<br/>
              2. 进入歌单页面，点击「分享」<br/>
              3. 选择「复制链接」<br/>
              4. 链接格式：<code>qishui.douyin.com/s/短链</code><br/>
              &nbsp;&nbsp;或 <code>music.douyin.com/qishui/share/playlist?playlist_id=数字</code>
            </Typography>
          </Box>
          {/* 提示 */}
          <Box style={{ padding: 12, border: '1px solid #bbdefb', borderRadius: 8, backgroundColor: '#e3f2fd' }}>
            <Typography style={{ fontWeight: 600, fontSize: 14, marginBottom: 8, color: '#1976d2' }}>
              💡 温馨提示
            </Typography>
            <Typography style={{ fontSize: 12, color: '#424242', lineHeight: 1.8 }}>
              • 粘贴链接后自动识别平台并采集歌曲列表<br/>
              • 采集到的歌曲会与本地曲库智能匹配<br/>
              • 支持模糊匹配（歌名包含即可）<br/>
              • 匹配成功的歌曲可一键创建为本地歌单<br/>
              • 网易云/QQ音乐同时支持采集歌单封面
            </Typography>
          </Box>
        </Box>
      </Card>
    </Box>
  )
}

export default AIPlaylistPage
