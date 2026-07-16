import React, { useState } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, Typography, Box, CircularProgress,
  Card, CardContent, Radio
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const ArtistAvatarDialog = ({ open, onClose, artist, onApply, searchType }) => {
  const [query, setQuery] = useState(artist?.name || '')
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState([])
  const [selected, setSelected] = useState(null)
  const [applying, setApplying] = useState(false)
  const [urlMode, setUrlMode] = useState(false)
  const [manualUrl, setManualUrl] = useState('')

  const handleSearch = async () => {
    if (!query.trim()) return
    setLoading(true)
    setResults([])
    setSelected(null)
    try {
      const typeParam = searchType ? `&type=${searchType}` : ''
      const res = await httpClient(`${REST_URL}/scrape/artist?q=${encodeURIComponent(query)}${typeParam}`)
      setResults(res.json?.data || [])
    } catch (e) {
      console.error('Artist search failed:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleApply = async () => {
    if (!artist) return
    setApplying(true)
    try {
      const imageUrl = urlMode ? manualUrl : selected?.imageUrl
      if (!imageUrl) {
        alert('请选择一张图片或输入URL')
        return
      }
      await httpClient(`${REST_URL}/scrape/artist/${artist.id}/avatar`, {
        method: 'POST',
        body: JSON.stringify({ imageUrl }),
      })
      if (onApply) onApply()
      onClose()
    } catch (e) {
      alert('保存失败: ' + e.message)
    } finally {
      setApplying(false)
    }
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>🔍 匹配艺人头像</DialogTitle>
      <DialogContent>
        <Box display="flex" gap={1} mb={2}>
          <TextField
            size="small" fullWidth variant="outlined"
            label="搜索艺人" value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
          />
          <Button variant="contained" color="primary" onClick={handleSearch}
            disabled={loading} startIcon={loading ? <CircularProgress size={16} /> : <SearchIcon />}>
            搜索
          </Button>
        </Box>

        {/* Toggle between search results and manual URL */}
        <Box display="flex" gap={1} mb={2}>
          <Button size="small" variant={!urlMode ? "contained" : "outlined"} onClick={() => setUrlMode(false)}>
            搜索结果
          </Button>
          <Button size="small" variant={urlMode ? "contained" : "outlined"} onClick={() => setUrlMode(true)}>
            输入URL
          </Button>
        </Box>

        {urlMode ? (
          <Box>
            <TextField
              size="small" fullWidth variant="outlined"
              label="图片URL" value={manualUrl}
              onChange={(e) => setManualUrl(e.target.value)}
              placeholder="https://example.com/avatar.jpg"
            />
            {manualUrl && (
              <Box mt={1} textAlign="center">
                <img src={manualUrl} alt="预览" style={{ maxWidth: 200, maxHeight: 200, borderRadius: 8 }}
                  onError={(e) => { e.target.style.display = 'none' }} />
              </Box>
            )}
          </Box>
        ) : (
          <Box>
            {results.length === 0 && !loading && (
              <Typography style={{ textAlign: 'center', color: '#999', padding: 20 }}>
                输入艺人名称搜索头像
              </Typography>
            )}
            {results.map((item, idx) => (
              <Card key={idx} style={{ marginBottom: 4, cursor: 'pointer',
                border: selected?.imageUrl === item.imageUrl ? '2px solid #1976d2' : '1px solid #e0e0e0'
              }} onClick={() => setSelected(item)}>
                <CardContent style={{ padding: '8px 12px', display: 'flex', gap: 12, alignItems: 'center' }}>
                  <Radio checked={selected?.imageUrl === item.imageUrl} />
                  <img src={item.imageUrl} alt="" style={{ width: 48, height: 48, borderRadius: '50%', objectFit: 'cover' }} />
                  <Box>
                    <Typography style={{ fontSize: 14, fontWeight: 600 }}>{item.name}</Typography>
                    <Typography style={{ fontSize: 11, color: '#666' }}>来源: {item.platform}</Typography>
                  </Box>
                </CardContent>
              </Card>
            ))}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>取消</Button>
        <Button onClick={handleApply} color="primary" variant="contained"
          disabled={applying || (!urlMode && !selected) || (urlMode && !manualUrl.trim())}>
          {applying ? '保存中...' : '保存头像'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default ArtistAvatarDialog
