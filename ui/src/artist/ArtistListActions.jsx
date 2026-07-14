import React, { cloneElement, useState } from 'react'
import { sanitizeListRestProps, TopToolbar, Button, useDataProvider, useNotify } from 'react-admin'
import { useMediaQuery, CircularProgress } from '@material-ui/core'
import { ToggleFieldsMenu } from '../common'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const ArtistListActions = ({
  className,
  filters,
  resource,
  showFilter,
  displayedFilters,
  filterValues,
  ...rest
}) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const [dialogOpen, setDialogOpen] = useState(false)

  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        })}
      {isNotSmall && <ToggleFieldsMenu resource="artist" />}
      <Button
        label="🔍 批量匹配头像"
        onClick={() => setDialogOpen(true)}
      />
      {dialogOpen && (
        <BatchAvatarDialog open={dialogOpen} onClose={() => setDialogOpen(false)} />
      )}
    </TopToolbar>
  )
}

const BatchAvatarDialog = ({ open, onClose }) => {
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(null)
  const [saving, setSaving] = useState(false)
  const [progress, setProgress] = useState({ current: 0, total: 0 })
  const notify = useNotify()

  const handleSearch = async () => {
    setLoading(true)
    setResults(null)
    try {
      // Get all artists using react-admin data provider format
      const res = await httpClient(`${REST_URL}/artist?sort=["name"]&order=ASC&range=[0,499]`)
      const artists = res.json || []
      setResults(Array.isArray(artists) ? artists : [])
    } catch (e) {
      console.error('Failed to load artists:', e)
      notify('加载艺人失败', 'warning')
    } finally {
      setLoading(false)
    }
  }

  const handleBatchSave = async () => {
    if (!results || results.length === 0) return
    setSaving(true)
    setProgress({ current: 0, total: results.length })

    let successCount = 0
    let failCount = 0

    for (let i = 0; i < results.length; i++) {
      const artist = results[i]
      setProgress({ current: i + 1, total: results.length })

      try {
        // Search for avatar
        const searchRes = await httpClient(`${REST_URL}/scrape/artist?q=${encodeURIComponent(artist.name)}`)
        const searchResults = searchRes.json?.data || []

        if (searchResults.length === 0) {
          failCount++
          continue
        }

        // Use the first result
        const avatar = searchResults[0]
        const saveRes = await httpClient(`${REST_URL}/scrape/artist/${encodeURIComponent(artist.id)}/avatar`, {
          method: 'POST',
          headers: new Headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify({ imageUrl: avatar.imageUrl }),
        })

        if (saveRes.json?.data?.imageUrl) {
          successCount++
        } else {
          failCount++
        }
      } catch (e) {
        console.error(`Failed to save avatar for ${artist.name}:`, e)
        failCount++
      }

      // Small delay to avoid overwhelming the server
      await new Promise(resolve => setTimeout(resolve, 500))
    }

    setSaving(false)
    notify(`完成: ${successCount} 成功, ${failCount} 失败`, successCount > 0 ? 'info' : 'warning')
    // Reload page to show new avatars
    if (successCount > 0) {
      setTimeout(() => window.location.reload(), 1000)
    }
  }

  if (!open) return null

  return (
    <div style={{
      position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
      backgroundColor: 'rgba(0,0,0,0.5)', zIndex: 9999,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }} onClick={onClose}>
      <div style={{
        backgroundColor: '#fff', borderRadius: 8, width: '90%', maxWidth: 600,
        maxHeight: '80vh', overflow: 'auto', padding: 24, color: '#333',
      }} onClick={e => e.stopPropagation()}>
        <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>🔍 批量匹配艺人头像</h2>

        <p style={{ fontSize: 13, color: '#666', marginBottom: 16 }}>
          自动为所有艺人搜索并保存头像。会从网易云音乐和QQ音乐搜索匹配的头像图片。
        </p>

        {!results && !loading && (
          <button onClick={handleSearch} style={{
            backgroundColor: '#1976d2', color: '#fff', border: 'none', borderRadius: 4,
            padding: '10px 24px', fontSize: 14, cursor: 'pointer',
          }}>
            加载艺人列表
          </button>
        )}

        {loading && (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <CircularProgress size={24} />
            <p>加载中...</p>
          </div>
        )}

        {results && !saving && (
          <div>
            <p style={{ fontWeight: 600 }}>共 {results.length} 位艺人</p>
            <div style={{ maxHeight: 300, overflow: 'auto', marginBottom: 16 }}>
              {results.map((artist, idx) => (
                <div key={artist.id} style={{
                  padding: '6px 12px', borderBottom: '1px solid #eee', fontSize: 13,
                }}>
                  {idx + 1}. {artist.name}
                </div>
              ))}
            </div>
            <button onClick={handleBatchSave} style={{
              backgroundColor: '#4caf50', color: '#fff', border: 'none', borderRadius: 4,
              padding: '10px 24px', fontSize: 14, cursor: 'pointer', marginRight: 8,
            }}>
              开始批量匹配
            </button>
            <button onClick={onClose} style={{
              backgroundColor: '#e0e0e0', border: 'none', borderRadius: 4,
              padding: '10px 24px', fontSize: 14, cursor: 'pointer',
            }}>
              取消
            </button>
          </div>
        )}

        {saving && (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <CircularProgress size={24} />
            <p>正在匹配头像 ({progress.current}/{progress.total})...</p>
            <div style={{
              width: '100%', height: 8, backgroundColor: '#e0e0e0', borderRadius: 4, marginTop: 8,
            }}>
              <div style={{
                width: `${(progress.current / progress.total) * 100}%`, height: '100%',
                backgroundColor: '#4caf50', borderRadius: 4, transition: 'width 0.3s',
              }} />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default ArtistListActions
