import React, { useState, useEffect } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, Typography, Box, Checkbox, FormControlLabel,
  CircularProgress, Chip, Tabs, Tab, Radio, RadioGroup,
  Card, CardContent, CardMedia, IconButton, Tooltip
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const ScrapeDialog = ({ open, onClose, book, onApply }) => {
  const [tab, setTab] = useState(0) // 0=单本刮削, 1=批量刮削
  const [query, setQuery] = useState('')
  const [sources, setSources] = useState([])
  const [loading, setLoading] = useState(false)
  const [searchResults, setSearchResults] = useState([])
  const [selectedResult, setSelectedResult] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [fields, setFields] = useState({
    title: false, author: false, narrator: false,
    description: false, genre: false, cover: false,
  })
  const [applying, setApplying] = useState(false)
  const [batchResults, setBatchResults] = useState(null)
  const [batchLoading, setBatchLoading] = useState(false)

  // Fetch available sources
  useEffect(() => {
    if (open) {
      httpClient(`${REST_URL}/scrape/sources`).then(res => {
        setSources(res.json?.data || [])
      }).catch(() => {})
      if (book) setQuery(book.title || '')
    }
  }, [open, book])

  // Search across all sources
  const handleSearch = async () => {
    if (!query.trim()) return
    setLoading(true)
    setSearchResults([])
    setSelectedResult(null)
    setDetail(null)
    try {
      const res = await httpClient(`${REST_URL}/scrape/audiobook?q=${encodeURIComponent(query)}`)
      setSearchResults(res.json?.data || [])
    } catch (e) {
      console.error('Scrape search failed:', e)
    } finally {
      setLoading(false)
    }
  }

  // Get detail for a specific result
  const handleSelectResult = async (source, id) => {
    setSelectedResult({ source, id })
    setDetailLoading(true)
    setDetail(null)
    try {
      const res = await httpClient(`${REST_URL}/scrape/audiobook/detail?source=${source}&id=${id}`)
      setDetail(res.json?.data || null)
    } catch (e) {
      console.error('Scrape detail failed:', e)
    } finally {
      setDetailLoading(false)
    }
  }

  // Apply selected fields
  const handleApply = async () => {
    if (!book || !detail) return
    setApplying(true)
    try {
      const body = {}
      if (fields.title && detail.title) body.title = detail.title
      if (fields.author && detail.author) body.author = detail.author
      if (fields.narrator && detail.narrator) body.narrator = detail.narrator
      if (fields.description && detail.intro) body.description = detail.intro
      if (fields.genre && detail.genre) body.genre = detail.genre
      if (fields.cover && detail.coverUrl) body.coverUrl = detail.coverUrl

      await httpClient(`${REST_URL}/scrape/audiobook/${book.id}/apply`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      if (onApply) onApply()
      onClose()
    } catch (e) {
      console.error('Apply failed:', e)
      alert('保存失败: ' + e.message)
    } finally {
      setApplying(false)
    }
  }

  // Batch scrape
  const handleBatchScrape = async () => {
    setBatchLoading(true)
    setBatchResults(null)
    try {
      const res = await httpClient(`${REST_URL}/scrape/batch`, {
        method: 'POST',
        body: JSON.stringify({ sources: sources.map(s => s.name), fields: ['title', 'author', 'narrator', 'cover'] }),
      })
      setBatchResults(res.json?.data || [])
    } catch (e) {
      console.error('Batch scrape failed:', e)
    } finally {
      setBatchLoading(false)
    }
  }

  // Flatten results from all sources
  const allResults = []
  for (const group of searchResults) {
    if (group.items) {
      group.items.forEach(item => allResults.push({ ...item, _source: group.source, _sourceName: group.name }))
    }
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>🔍 刮削有声书信息</DialogTitle>
      <DialogContent>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} style={{ marginBottom: 16 }}>
          <Tab label="单本刮削" />
          <Tab label="批量刮削" />
        </Tabs>

        {/* Single Book Scrape */}
        {tab === 0 && (
          <Box>
            <Box display="flex" gap={1} mb={2}>
              <TextField
                size="small" fullWidth variant="outlined"
                label="搜索关键词" value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyPress={(e) => e.key === 'Enter' && handleSearch()}
                placeholder="输入书名、作者或演播者"
              />
              <Button variant="contained" color="primary" onClick={handleSearch}
                disabled={loading} startIcon={loading ? <CircularProgress size={16} /> : <SearchIcon />}>
                搜索
              </Button>
            </Box>

            {/* Source chips */}
            <Box mb={1}>
              {sources.map(s => (
                <Chip key={s.name} label={s.displayName} size="small" style={{ marginRight: 4 }} />
              ))}
            </Box>

            {/* Search Results */}
            {allResults.length > 0 && (
              <Box mb={2}>
                <Typography variant="subtitle2" gutterBottom>搜索结果 (点击选择)</Typography>
                <Box style={{ maxHeight: 200, overflow: 'auto' }}>
                  {allResults.map((item, idx) => (
                    <Card key={idx} style={{ marginBottom: 4, cursor: 'pointer',
                      border: selectedResult?.source === item._source && selectedResult?.id === item.id ? '2px solid #1976d2' : '1px solid #e0e0e0'
                    }} onClick={() => handleSelectResult(item._source, item.id)}>
                      <CardContent style={{ padding: '8px 12px', display: 'flex', gap: 12, alignItems: 'center' }}>
                        {item.coverUrl && (
                          <img src={item.coverUrl} alt="" style={{ width: 40, height: 40, objectFit: 'cover', borderRadius: 4 }} />
                        )}
                        <Box flex={1}>
                          <Typography style={{ fontSize: 13, fontWeight: 600 }}>{item.title}</Typography>
                          <Typography style={{ fontSize: 11, color: '#666' }}>
                            {item.author && `作者: ${item.author}`}
                            {item.narrator && ` | 演播: ${item.narrator}`}
                            {item.chapterCount > 0 && ` | ${item.chapterCount}章`}
                          </Typography>
                        </Box>
                        <Chip label={item._sourceName || item._source} size="small" variant="outlined" />
                      </CardContent>
                    </Card>
                  ))}
                </Box>
              </Box>
            )}

            {/* Detail View */}
            {detailLoading && <CircularProgress size={24} />}
            {detail && (
              <Box>
                <Typography variant="subtitle2" gutterBottom>刮削详情 (勾选要保留的字段)</Typography>
                <Box style={{ border: '1px solid #e0e0e0', borderRadius: 8, padding: 12 }}>
                  <Box display="flex" gap={2} mb={2}>
                    {detail.coverUrl && (
                      <img src={detail.coverUrl} alt="" style={{ width: 100, height: 100, objectFit: 'cover', borderRadius: 8 }} />
                    )}
                    <Box>
                      <Typography style={{ fontSize: 16, fontWeight: 700 }}>{detail.title}</Typography>
                      {detail.author && <Typography style={{ fontSize: 13 }}>作者: {detail.author}</Typography>}
                      {detail.narrator && <Typography style={{ fontSize: 13 }}>演播者: {detail.narrator}</Typography>}
                      {detail.genre && <Typography style={{ fontSize: 13 }}>分类: {detail.genre}</Typography>}
                      {detail.chapterCount > 0 && <Typography style={{ fontSize: 13 }}>章节数: {detail.chapterCount}</Typography>}
                    </Box>
                  </Box>
                  {detail.intro && (
                    <Typography style={{ fontSize: 12, color: '#666', maxHeight: 100, overflow: 'auto', marginBottom: 8 }}>
                      {detail.intro}
                    </Typography>
                  )}

                  {/* Field checkboxes */}
                  <Box display="flex" flexWrap="wrap" gap={1}>
                    {detail.title && <FormControlLabel control={<Checkbox checked={fields.title} onChange={(e) => setFields({...fields, title: e.target.checked})} />} label="标题" />}
                    {detail.author && <FormControlLabel control={<Checkbox checked={fields.author} onChange={(e) => setFields({...fields, author: e.target.checked})} />} label="作者" />}
                    {detail.narrator && <FormControlLabel control={<Checkbox checked={fields.narrator} onChange={(e) => setFields({...fields, narrator: e.target.checked})} />} label="演播者" />}
                    {detail.intro && <FormControlLabel control={<Checkbox checked={fields.description} onChange={(e) => setFields({...fields, description: e.target.checked})} />} label="简介" />}
                    {detail.genre && <FormControlLabel control={<Checkbox checked={fields.genre} onChange={(e) => setFields({...fields, genre: e.target.checked})} />} label="分类" />}
                    {detail.coverUrl && <FormControlLabel control={<Checkbox checked={fields.cover} onChange={(e) => setFields({...fields, cover: e.target.checked})} />} label="封面" />}
                  </Box>
                </Box>
              </Box>
            )}
          </Box>
        )}

        {/* Batch Scrape */}
        {tab === 1 && (
          <Box>
            <Typography variant="body2" gutterBottom>
              批量刮削将搜索所有有声书的元数据。你可以选择保留哪些字段。
            </Typography>
            <Button variant="contained" color="primary" onClick={handleBatchScrape}
              disabled={batchLoading} startIcon={batchLoading ? <CircularProgress size={16} /> : <SearchIcon />}
              style={{ marginTop: 8 }}>
              {batchLoading ? '刮削中...' : '开始批量刮削'}
            </Button>
            {batchResults && (
              <Box mt={2}>
                <Typography variant="subtitle2">找到 {batchResults.length} 本有声书的匹配结果</Typography>
                <Box style={{ maxHeight: 400, overflow: 'auto' }}>
                  {batchResults.map((item, idx) => (
                    <Card key={idx} style={{ marginBottom: 8 }}>
                      <CardContent style={{ padding: 8 }}>
                        <Typography style={{ fontWeight: 600 }}>{item.title}</Typography>
                        {Object.entries(item.results || {}).map(([source, results]) => (
                          <Box key={source} ml={1}>
                            <Typography style={{ fontSize: 11, color: '#999' }}>{source}:</Typography>
                            {(results || []).slice(0, 2).map((r, i) => (
                              <Typography key={i} style={{ fontSize: 12, marginLeft: 8 }}>
                                • {r.title} {r.author && `(${r.author})`} {r.narrator && `[${r.narrator}]`}
                              </Typography>
                            ))}
                          </Box>
                        ))}
                      </CardContent>
                    </Card>
                  ))}
                </Box>
              </Box>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>取消</Button>
        {tab === 0 && detail && (
          <Button onClick={handleApply} color="primary" variant="contained"
            disabled={applying || Object.values(fields).every(v => !v)}>
            {applying ? '保存中...' : '保存选中字段'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  )
}

export default ScrapeDialog
