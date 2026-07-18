import React, { useState, useEffect } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, TextField, Typography, Box, Checkbox, FormControlLabel,
  CircularProgress, Chip, Tabs, Tab,
  Card, CardContent, Table, TableBody, TableCell, TableContainer,
  TableHead, TableRow, Paper, IconButton, Tooltip, LinearProgress,
} from '@material-ui/core'
import SearchIcon from '@material-ui/icons/Search'
import CheckCircleIcon from '@material-ui/icons/CheckCircle'
import ErrorIcon from '@material-ui/icons/Error'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const ScrapeDialog = ({ open, onClose, book, onApply }) => {
  const [tab, setTab] = useState(0)
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
  // Batch field selection
  const [batchFields, setBatchFields] = useState({
    title: true, author: true, narrator: true, cover: true, genre: false, description: false,
  })
  // Batch selected books
  const [batchSelected, setBatchSelected] = useState({})
  // Batch apply progress
  const [batchProgress, setBatchProgress] = useState(null)
  const [batchApplying, setBatchApplying] = useState(false)

  useEffect(() => {
    if (open) {
      httpClient(`${REST_URL}/scrape/sources`).then(res => {
        setSources(res.json?.data || [])
      }).catch(() => {})
      if (book) setQuery(book.title || '')
      // Reset batch state
      setBatchResults(null)
      setBatchSelected({})
      setBatchProgress(null)
    }
  }, [open, book])

  // ─── Single Book Scrape ─────────────────────────────────

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

  const handleSelectResult = async (source, id, searchItem) => {
    setSelectedResult({ source, id })
    setDetailLoading(true)
    setDetail(null)
    try {
      const res = await httpClient(`${REST_URL}/scrape/audiobook/detail?source=${source}&id=${id}`)
      const detailData = res.json?.data || {}
      setDetail({
        title: detailData.title || searchItem?.title || '',
        author: detailData.author || searchItem?.author || '',
        narrator: detailData.narrator || searchItem?.narrator || '',
        coverUrl: detailData.coverUrl || searchItem?.coverUrl || '',
        intro: detailData.intro || searchItem?.intro || '',
        genre: detailData.genre || searchItem?.genre || '',
        year: detailData.year || searchItem?.year || 0,
        chapterCount: detailData.chapterCount || searchItem?.chapterCount || 0,
      })
    } catch (e) {
      console.error('Scrape detail failed:', e)
    } finally {
      setDetailLoading(false)
    }
  }

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
      if (selectedResult) {
        body.source = selectedResult.source
        body.sourceId = selectedResult.id
      }
      await httpClient(`${REST_URL}/scrape/audiobook/${book.id}/apply`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      if (onApply) onApply()
      onClose()
    } catch (e) {
      alert('保存失败: ' + e.message)
    } finally {
      setApplying(false)
    }
  }

  // ─── Batch Scrape ───────────────────────────────────────

  const handleBatchScrape = async () => {
    setBatchLoading(true)
    setBatchResults(null)
    setBatchSelected({})
    setBatchProgress(null)
    try {
      const res = await httpClient(`${REST_URL}/scrape/batch`, {
        method: 'POST',
        body: JSON.stringify({ sources: sources.map(s => s.name), fields: ['title', 'author', 'narrator', 'cover'] }),
      })
      const results = res.json?.data || []
      setBatchResults(results)
      // Auto-select all books
      const selected = {}
      results.forEach(item => { selected[item.bookId] = true })
      setBatchSelected(selected)
    } catch (e) {
      console.error('Batch scrape failed:', e)
    } finally {
      setBatchLoading(false)
    }
  }

  // Get the best result for a book (first available from any source)
  const getBestResult = (item) => {
    for (const [, results] of Object.entries(item.results || {})) {
      if (results && results.length > 0) {
        return results[0]
      }
    }
    return null
  }

  const handleBatchToggleBook = (bookId) => {
    setBatchSelected(prev => ({ ...prev, [bookId]: !prev[bookId] }))
  }

  const handleBatchSelectAll = (checked) => {
    const selected = {}
    if (checked) {
      batchResults.forEach(item => { selected[item.bookId] = true })
    }
    setBatchSelected(selected)
  }

  const handleBatchApply = async () => {
    const selectedBooks = batchResults.filter(item => batchSelected[item.bookId])
    if (selectedBooks.length === 0) {
      alert('请至少选择一本有声书')
      return
    }
    const activeFields = Object.entries(batchFields).filter(([, v]) => v).map(([k]) => k)
    if (activeFields.length === 0) {
      alert('请至少选择一个字段')
      return
    }

    setBatchApplying(true)
    setBatchProgress({ total: selectedBooks.length, done: 0, success: 0, fail: 0, current: '' })

    for (let i = 0; i < selectedBooks.length; i++) {
      const item = selectedBooks[i]
      const result = getBestResult(item)
      if (!result) {
        setBatchProgress(prev => ({ ...prev, done: i + 1, fail: prev.fail + 1, current: item.title }))
        continue
      }

      setBatchProgress(prev => ({ ...prev, current: item.title }))
      try {
        const body = {}
        if (activeFields.includes('title') && result.title) body.title = result.title
        if (activeFields.includes('author') && result.author) body.author = result.author
        if (activeFields.includes('narrator') && result.narrator) body.narrator = result.narrator
        if (activeFields.includes('description') && result.intro) body.description = result.intro
        if (activeFields.includes('genre') && result.genre) body.genre = result.genre
        if (activeFields.includes('cover') && result.coverUrl) body.coverUrl = result.coverUrl

        await httpClient(`${REST_URL}/scrape/audiobook/${item.bookId}/apply`, {
          method: 'POST',
          body: JSON.stringify(body),
        })
        setBatchProgress(prev => ({ ...prev, done: i + 1, success: prev.success + 1 }))
      } catch (e) {
        setBatchProgress(prev => ({ ...prev, done: i + 1, fail: prev.fail + 1 }))
      }
    }

    setBatchApplying(false)
    if (onApply) onApply()
  }

  const allResults = []
  for (const group of searchResults) {
    if (group.items) {
      group.items.forEach(item => allResults.push({ ...item, _source: group.source, _sourceName: group.name }))
    }
  }

  const selectedCount = Object.values(batchSelected).filter(Boolean).length

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>🔍 刮削有声书信息</DialogTitle>
      <DialogContent>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} style={{ marginBottom: 16 }}>
          <Tab label="单本刮削" />
          <Tab label="批量刮削" />
        </Tabs>

        {/* ═══════════ Single Book Scrape ═══════════ */}
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

            <Box mb={1}>
              {sources.map(s => (
                <Chip key={s.name} label={s.displayName} size="small" style={{ marginRight: 4 }} />
              ))}
            </Box>

            {allResults.length > 0 && (
              <Box mb={2}>
                <Typography variant="subtitle2" gutterBottom>搜索结果 (点击选择)</Typography>
                <Box style={{ maxHeight: 200, overflow: 'auto' }}>
                  {allResults.map((item, idx) => (
                    <Card key={idx} style={{ marginBottom: 4, cursor: 'pointer',
                      border: selectedResult?.source === item._source && selectedResult?.id === item.id ? '2px solid #1976d2' : '1px solid #e0e0e0'
                    }} onClick={() => handleSelectResult(item._source, item.id, item)}>
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

        {/* ═══════════ Batch Scrape ═══════════ */}
        {tab === 1 && (
          <Box>
            {!batchResults && !batchLoading && (
              <Box textAlign="center" py={3}>
                <Typography variant="body2" color="textSecondary" gutterBottom>
                  批量刮削将搜索所有有声书的元数据，展示匹配结果后你可以选择哪些书要更新哪些字段。
                </Typography>
                <Button variant="contained" color="primary" onClick={handleBatchScrape}
                  startIcon={<SearchIcon />} style={{ marginTop: 12 }}>
                  开始批量刮削
                </Button>
              </Box>
            )}

            {batchLoading && (
              <Box textAlign="center" py={4}>
                <CircularProgress />
                <Typography style={{ marginTop: 12 }} color="textSecondary">正在搜索所有有声书的元数据...</Typography>
              </Box>
            )}

            {/* Batch Progress */}
            {batchApplying && batchProgress && (
              <Box mb={2}>
                <Typography variant="subtitle2" gutterBottom>
                  正在保存... {batchProgress.done}/{batchProgress.total}
                  {batchProgress.success > 0 && <span style={{ color: '#4caf50' }}> ✓{batchProgress.success}</span>}
                  {batchProgress.fail > 0 && <span style={{ color: '#f44336' }}> ✗{batchProgress.fail}</span>}
                </Typography>
                <LinearProgress variant="determinate" value={(batchProgress.done / batchProgress.total) * 100} />
                <Typography style={{ fontSize: 12, color: '#999', marginTop: 4 }}>
                  当前: {batchProgress.current}
                </Typography>
              </Box>
            )}

            {/* Batch Results */}
            {batchResults && !batchApplying && (
              <Box>
                {/* Field Selection Bar */}
                <Box mb={2} p={1.5} style={{ backgroundColor: '#f5f5f5', borderRadius: 8 }}>
                  <Typography variant="subtitle2" gutterBottom style={{ fontSize: 13 }}>
                    要应用的字段（勾选后批量保存时生效）：
                  </Typography>
                  <Box display="flex" flexWrap="wrap" gap={0}>
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.title} onChange={(e) => setBatchFields({...batchFields, title: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>标题</span>} />
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.author} onChange={(e) => setBatchFields({...batchFields, author: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>作者</span>} />
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.narrator} onChange={(e) => setBatchFields({...batchFields, narrator: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>演播者</span>} />
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.genre} onChange={(e) => setBatchFields({...batchFields, genre: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>分类</span>} />
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.description} onChange={(e) => setBatchFields({...batchFields, description: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>简介</span>} />
                    <FormControlLabel control={<Checkbox size="small" checked={batchFields.cover} onChange={(e) => setBatchFields({...batchFields, cover: e.target.checked})} />} label={<span style={{ fontSize: 13 }}>封面</span>} />
                  </Box>
                </Box>

                {/* Select All / Count */}
                <Box display="flex" alignItems="center" justifyContent="space-between" mb={1}>
                  <Box display="flex" alignItems="center" gap={1}>
                    <Checkbox
                      size="small"
                      checked={selectedCount === batchResults.length}
                      indeterminate={selectedCount > 0 && selectedCount < batchResults.length}
                      onChange={(e) => handleBatchSelectAll(e.target.checked)}
                    />
                    <Typography variant="subtitle2" style={{ fontSize: 13 }}>
                      全选 | 已选 {selectedCount}/{batchResults.length}
                    </Typography>
                  </Box>
                  <Button size="small" color="primary" variant="contained"
                    disabled={selectedCount === 0}
                    onClick={handleBatchApply}>
                    批量保存所选
                  </Button>
                </Box>

                {/* Results Table */}
                <TableContainer component={Paper} style={{ maxHeight: 450, overflow: 'auto' }}>
                  <Table size="small" stickyHeader>
                    <TableHead>
                      <TableRow>
                        <TableCell padding="checkbox" style={{ width: 40 }}></TableCell>
                        <TableCell style={{ minWidth: 120 }}>原书名</TableCell>
                        <TableCell style={{ minWidth: 100 }}>封面</TableCell>
                        <TableCell style={{ minWidth: 120 }}>刮削标题</TableCell>
                        <TableCell style={{ minWidth: 80 }}>作者</TableCell>
                        <TableCell style={{ minWidth: 80 }}>演播者</TableCell>
                        <TableCell style={{ minWidth: 60 }}>分类</TableCell>
                        <TableCell style={{ minWidth: 60 }}>来源</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {batchResults.map((item, idx) => {
                        const result = getBestResult(item)
                        const isSelected = !!batchSelected[item.bookId]
                        return (
                          <TableRow key={idx} hover selected={isSelected}
                            style={{ cursor: 'pointer', opacity: result ? 1 : 0.5 }}
                            onClick={() => result && handleBatchToggleBook(item.bookId)}>
                            <TableCell padding="checkbox">
                              <Checkbox size="small" checked={isSelected} disabled={!result} />
                            </TableCell>
                            <TableCell>
                              <Typography style={{ fontSize: 13, fontWeight: 500 }}>{item.title}</Typography>
                            </TableCell>
                            <TableCell>
                              {result?.coverUrl ? (
                                <img src={result.coverUrl} alt="" style={{ width: 48, height: 48, objectFit: 'cover', borderRadius: 4 }} />
                              ) : (
                                <Box style={{ width: 48, height: 48, backgroundColor: '#eee', borderRadius: 4, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                  <Typography style={{ fontSize: 10, color: '#999' }}>无</Typography>
                                </Box>
                              )}
                            </TableCell>
                            <TableCell>
                              {result?.title ? (
                                <Typography style={{ fontSize: 13 }}>{result.title}</Typography>
                              ) : (
                                <Typography style={{ fontSize: 12, color: '#999' }}>未匹配</Typography>
                              )}
                            </TableCell>
                            <TableCell>
                              <Typography style={{ fontSize: 13 }}>{result?.author || '-'}</Typography>
                            </TableCell>
                            <TableCell>
                              <Typography style={{ fontSize: 13 }}>{result?.narrator || '-'}</Typography>
                            </TableCell>
                            <TableCell>
                              {result?.genre && <Chip label={result.genre} size="small" style={{ fontSize: 10, height: 20 }} />}
                            </TableCell>
                            <TableCell>
                              {Object.keys(item.results || {}).map(src => (
                                <Chip key={src} label={src} size="small" variant="outlined"
                                  style={{ fontSize: 9, height: 18, marginRight: 2 }} />
                              ))}
                            </TableCell>
                          </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                </TableContainer>

                {/* Summary after apply */}
                {batchProgress && batchProgress.done === batchProgress.total && (
                  <Box mt={2} textAlign="center">
                    <Typography style={{ color: '#4caf50', fontWeight: 600 }}>
                      ✓ 完成！成功 {batchProgress.success} 本，失败 {batchProgress.fail} 本
                    </Typography>
                  </Box>
                )}
              </Box>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{batchApplying ? '后台继续' : '取消'}</Button>
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
