import React, { useState, useCallback } from 'react'
import {
  Dialog, DialogTitle, DialogContent, DialogActions,
  Button, Typography, Box, CircularProgress, Chip,
  Accordion, AccordionSummary, AccordionDetails, IconButton, Tooltip,
} from '@material-ui/core'
import ExpandMoreIcon from '@material-ui/icons/ExpandMore'
import DeleteIcon from '@material-ui/icons/Delete'
import FolderOpenIcon from '@material-ui/icons/FolderOpen'
import FileCopyIcon from '@material-ui/icons/FileCopy'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const DuplicateSongsDialog = ({ open, onClose }) => {
  const [loading, setLoading] = useState(false)
  const [duplicates, setDuplicates] = useState(null)
  const [error, setError] = useState(null)
  const [selectedIds, setSelectedIds] = useState(new Set())
  const [deleting, setDeleting] = useState(false)

  const handleScan = useCallback(async () => {
    setLoading(true)
    setError(null)
    setDuplicates(null)
    try {
      const res = await httpClient(`${REST_URL}/song/duplicates`)
      setDuplicates(res.json?.data || [])
    } catch (e) {
      console.error('Failed to find duplicates:', e)
      setError(e.message || '扫描失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const handleCopyPath = useCallback((path) => {
    if (navigator.clipboard) {
      navigator.clipboard.writeText(path).then(() => {
        // silent success
      }).catch(() => {})
    }
  }, [])

  const toggleSelect = useCallback((id) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const selectAllDuplicates = useCallback(() => {
    if (!duplicates) return
    const ids = new Set()
    duplicates.forEach(group => {
      // Select all except the first (original) in each group
      group.songs.slice(1).forEach(s => ids.add(s.id))
    })
    setSelectedIds(ids)
  }, [duplicates])

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  const handleDeleteSingle = useCallback(async (songId, songPath) => {
    if (!confirm(`确定要删除此文件吗？\n${songPath}\n\n此操作不可撤销！`)) return
    try {
      const res = await httpClient(`${REST_URL}/song/duplicates/delete`, {
        method: 'POST',
        body: JSON.stringify({ ids: [songId] }),
      })
      const data = res.json || {}
      if (res.status >= 200 && res.status < 300 && data.deleted > 0) {
        setDuplicates(prev => prev.map(g => ({
          ...g,
          songs: g.songs.filter(s => s.id !== songId),
          count: g.songs.filter(s => s.id !== songId).length,
        })).filter(g => g.songs.length > 1))
        if (data.errors?.length > 0) {
          alert('部分删除失败:\n' + data.errors.join('\n'))
        }
      } else {
        alert('删除失败: ' + (data.errors?.join('\n') || res.statusText || '未知错误'))
      }
    } catch (e) {
      alert('删除失败: ' + e.message)
    }
  }, [])

  const handleDeleteSelected = useCallback(async () => {
    if (selectedIds.size === 0) return
    if (!confirm(`确定要删除选中的 ${selectedIds.size} 个文件吗？\n\n此操作不可撤销！`)) return
    setDeleting(true)
    try {
      const ids = Array.from(selectedIds)
      const res = await httpClient(`${REST_URL}/song/duplicates/delete`, {
        method: 'POST',
        body: JSON.stringify({ ids }),
      })
      const data = res.json || {}
      if (res.status >= 200 && res.status < 300) {
        const deletedSet = new Set(ids)
        setDuplicates(prev => prev.map(g => ({
          ...g,
          songs: g.songs.filter(s => !deletedSet.has(s.id)),
          count: g.songs.filter(s => !deletedSet.has(s.id)).length,
        })).filter(g => g.songs.length > 1))
        setSelectedIds(new Set())
        if (data.deleted > 0) {
          alert(`成功删除 ${data.deleted} 个文件${data.failed > 0 ? '，' + data.failed + ' 个失败' : ''}`)
        }
        if (data.errors?.length > 0) {
          alert('删除详情:\n' + data.errors.join('\n'))
        }
      } else {
        alert('删除失败: ' + (data.errors?.join('\n') || res.statusText || '未知错误'))
      }
    } catch (e) {
      alert('删除失败: ' + e.message)
    } finally {
      setDeleting(false)
    }
  }, [selectedIds])

  const totalDuplicates = duplicates ? duplicates.reduce((sum, g) => sum + g.count, 0) : 0
  const wastedSize = duplicates ? duplicates.reduce((sum, g) => {
    // 每组中除第一个外的文件算作冗余
    return sum + g.songs.slice(1).reduce((s, song) => s + (song.size || 0), 0)
  }, 0) : 0

  const formatSize = (bytes) => {
    if (!bytes) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB']
    let i = 0
    let size = bytes
    while (size >= 1024 && i < units.length - 1) {
      size /= 1024
      i++
    }
    return `${size.toFixed(1)} ${units[i]}`
  }

  const formatDuration = (seconds) => {
    if (!seconds) return ''
    const m = Math.floor(seconds / 60)
    const s = Math.floor(seconds % 60)
    return `${m}:${s.toString().padStart(2, '0')}`
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>🔍 重复歌曲检测</DialogTitle>
      <DialogContent>
        <Box mb={2}>
          <Typography variant="body2" color="textSecondary" gutterBottom>
            扫描歌曲库，按 标题+艺术家 筛选重复的歌曲。结果中会显示每个文件的路径，方便你手动删除重复文件。
          </Typography>
          <Box style={{ display: 'flex', gap: 8, marginTop: 8 }}>
            <Button
              variant="contained"
              color="primary"
              onClick={handleScan}
              disabled={loading}
              startIcon={loading ? <CircularProgress size={16} /> : <FileCopyIcon />}
            >
              {loading ? '扫描中...' : '开始扫描'}
            </Button>
            {duplicates && duplicates.length > 0 && (
              <>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={selectAllDuplicates}
                  style={{ textTransform: 'none' }}
                >
                  ☑ 全选重复项
                </Button>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={clearSelection}
                  style={{ textTransform: 'none' }}
                >
                  取消全选
                </Button>
              </>
            )}
          </Box>
        </Box>

        {error && (
          <Box mb={2} p={2} style={{ backgroundColor: '#fff3f3', borderRadius: 8 }}>
            <Typography color="error">{error}</Typography>
          </Box>
        )}

        {duplicates && (
          <Box>
            {/* 统计信息 */}
            <Box mb={2} p={2} style={{ backgroundColor: '#e3f2fd', borderRadius: 8, display: 'flex', gap: 24, flexWrap: 'wrap', alignItems: 'center' }}>
              <Typography variant="body2" style={{ color: '#1565c0', fontWeight: 600 }}>
                重复组数：{duplicates.length}
              </Typography>
              <Typography variant="body2" style={{ color: '#1565c0', fontWeight: 600 }}>
                涉及歌曲：{totalDuplicates} 首
              </Typography>
              <Typography variant="body2" style={{ color: '#1565c0', fontWeight: 600 }}>
                估算冗余空间：{formatSize(wastedSize)}
              </Typography>
              {selectedIds.size > 0 && (
                <Button
                  size="small"
                  variant="contained"
                  color="secondary"
                  startIcon={<DeleteIcon />}
                  onClick={handleDeleteSelected}
                  disabled={deleting}
                  style={{ marginLeft: 'auto', textTransform: 'none' }}
                >
                  {deleting ? '删除中...' : `删除选中 (${selectedIds.size})`}
                </Button>
              )}
            </Box>

            {duplicates.length === 0 ? (
              <Box textAlign="center" py={4}>
                <Typography color="textSecondary">🎉 未发现重复歌曲</Typography>
              </Box>
            ) : (
              <Box style={{ maxHeight: 500, overflow: 'auto' }}>
                {duplicates.map((group, idx) => (
                  <Accordion key={idx} defaultExpanded={idx < 3}>
                    <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                      <Box display="flex" alignItems="center" gap={1} flex={1}>
                        <Typography style={{ fontWeight: 600, flex: 1 }}>
                          {group.title}
                        </Typography>
                        {group.artist && (
                          <Typography style={{ color: '#666', fontSize: 13 }}>
                            {group.artist}
                          </Typography>
                        )}
                        <Chip
                          label={`${group.count} 个文件`}
                          size="small"
                          color={group.count > 2 ? 'secondary' : 'default'}
                        />
                      </Box>
                    </AccordionSummary>
                    <AccordionDetails>
                      <Box width="100%">
                        {group.songs.map((song, songIdx) => (
                          <Box
                            key={song.id}
                            p={1.5}
                            mb={0.5}
                            style={{
                              backgroundColor: songIdx === 0 ? '#e8f5e9' : '#fff3e0',
                              borderRadius: 6,
                              border: songIdx === 0 ? '1px solid #a5d6a7' : '1px solid #ffcc80',
                            }}
                          >
                            <Box display="flex" alignItems="flex-start" gap={1}>
                              {songIdx > 0 && (
                                <input
                                  type="checkbox"
                                  checked={selectedIds.has(song.id)}
                                  onChange={() => toggleSelect(song.id)}
                                  style={{ marginTop: 4, cursor: 'pointer' }}
                                />
                              )}
                              <Box flex={1}>
                                <Box display="flex" alignItems="center" gap={1} mb={0.5}>
                                  <Chip
                                    label={songIdx === 0 ? '原始' : `重复 ${songIdx}`}
                                    size="small"
                                    style={{
                                      fontSize: 11,
                                      height: 20,
                                      backgroundColor: songIdx === 0 ? '#4caf50' : '#ff9800',
                                      color: 'white',
                                    }}
                                  />
                                  <Typography style={{ fontSize: 12, color: '#666' }}>
                                    {song.album && `专辑: ${song.album}`}
                                    {song.year > 0 && ` · ${song.year}`}
                                    {song.duration > 0 && ` · ${formatDuration(song.duration)}`}
                                    {song.bitRate > 0 && ` · ${song.bitRate}kbps`}
                                    {song.suffix && ` · ${song.suffix.toUpperCase()}`}
                                  </Typography>
                                </Box>
                                {/* 文件路径 - 重点展示 */}
                                <Box
                                  display="flex"
                                  alignItems="center"
                                  gap={0.5}
                                  style={{
                                    fontFamily: 'monospace',
                                    fontSize: 12,
                                    color: '#333',
                                    backgroundColor: 'rgba(0,0,0,0.04)',
                                    padding: '4px 8px',
                                    borderRadius: 4,
                                    wordBreak: 'break-all',
                                  }}
                                >
                                  <FolderOpenIcon style={{ fontSize: 14, color: '#666', flexShrink: 0 }} />
                                  <span>{song.path || '路径未知'}</span>
                                  <Tooltip title="复制路径">
                                    <IconButton
                                      size="small"
                                      onClick={(e) => { e.stopPropagation(); handleCopyPath(song.path) }}
                                      style={{ padding: 2 }}
                                    >
                                      <FileCopyIcon style={{ fontSize: 14 }} />
                                    </IconButton>
                                  </Tooltip>
                                  {songIdx > 0 && (
                                    <Tooltip title="删除此文件">
                                      <IconButton
                                        size="small"
                                        onClick={(e) => { e.stopPropagation(); handleDeleteSingle(song.id, song.path) }}
                                        style={{ padding: 2, color: '#f44336' }}
                                      >
                                        <DeleteIcon style={{ fontSize: 14 }} />
                                      </IconButton>
                                    </Tooltip>
                                  )}
                                </Box>
                              </Box>
                            </Box>
                          </Box>
                        ))}
                      </Box>
                    </AccordionDetails>
                  </Accordion>
                ))}
              </Box>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>关闭</Button>
      </DialogActions>
    </Dialog>
  )
}

export default DuplicateSongsDialog
