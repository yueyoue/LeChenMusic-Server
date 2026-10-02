import React, { cloneElement, useState } from 'react'
import {
  sanitizeListRestProps,
  TopToolbar,
  Button,
  useListContext,
  useNotify,
} from 'react-admin'
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
  // 以 useListContext 的实时值为准：react-admin 不一定会把 filterValues 传给 actions 元素，
  // 拿 props 里的值会是 undefined，导致批量弹窗发出去的是空 filter（列出全部艺人）。
  const {
    setFilters,
    filterValues: ctxFilterValues,
    displayedFilters: ctxDisplayedFilters,
  } = useListContext()
  const activeFilters = ctxFilterValues ?? filterValues ?? {}
  const noImageOnly = String(activeFilters?.no_image) === 'true'

  // 把所有未设置头像（显示默认头像）的艺人筛选出来：往列表过滤器里加 no_image=true。
  // 后端 persistence.noImageFilter 按「没有任何头像图源」筛选。
  const toggleNoImageFilter = () => {
    const next = { ...activeFilters }
    if (noImageOnly) {
      delete next.no_image
    } else {
      next.no_image = 'true'
    }
    setFilters(next, ctxDisplayedFilters ?? displayedFilters ?? {})
  }

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
        label={noImageOnly ? '🖼️ 显示全部艺人' : '🖼️ 筛选无头像'}
        onClick={toggleNoImageFilter}
      />
      <Button
        label={noImageOnly ? '🔍 批量匹配无头像' : '🔍 批量匹配头像'}
        onClick={() => setDialogOpen(true)}
      />
      {dialogOpen && (
        <BatchAvatarDialog
          open={dialogOpen}
          onClose={() => setDialogOpen(false)}
          filterValues={activeFilters}
          noImageOnly={noImageOnly}
        />
      )}
    </TopToolbar>
  )
}

// 艺人是否已经有头像（与服务端 noImageFilter 同口径：没有任何头像图源就是显示默认头像）。
// 批量弹窗里再自己兜一次底，即使服务端过滤器没生效也只处理真正没头像的艺人。
const hasAvatar = (a) =>
  !!(
    a?.smallImageUrl ||
    a?.mediumImageUrl ||
    a?.largeImageUrl ||
    a?.uploadedImage
  )

const BatchAvatarDialog = ({ open, onClose, filterValues, noImageOnly }) => {
  const [loading, setLoading] = useState(false)
  const [results, setResults] = useState(null)
  const [saving, setSaving] = useState(false)
  const [progress, setProgress] = useState({ current: 0, total: 0 })
  const notify = useNotify()

  const handleSearch = async () => {
    setLoading(true)
    setResults(null)
    try {
      // 参数约定：后端走 deluan/rest，认的是 _start/_end/_sort/_order + 「拍平的过滤条件」，
      // 不认 ra-data-simple-rest 那套 filter=/range=/sort=（那些会被当成普通过滤字段丢掉，
      // 所以以前弹窗永远列出全部艺人）。这里跟列表页发的请求保持同一套约定。
      const byId = new Map()
      const pageSize = 500
      for (let page = 0; page < 20; page++) {
        const qs = new URLSearchParams()
        qs.set('_start', String(page * pageSize))
        qs.set('_end', String((page + 1) * pageSize))
        qs.set('_sort', 'name')
        qs.set('_order', 'ASC')
        Object.entries(filterValues || {}).forEach(([key, value]) => {
          if (value === undefined || value === null || value === '') return
          if (Array.isArray(value)) {
            value.forEach((v) => qs.append(key, String(v)))
          } else {
            qs.set(key, String(value))
          }
        })
        const res = await httpClient(`${REST_URL}/artist?${qs.toString()}`)
        const batch = res.json || []
        const list = Array.isArray(batch) ? batch : []
        if (list.length === 0) break
        const before = byId.size
        list.forEach((a) => a?.id && byId.set(a.id, a))
        if (byId.size === before) break // 没有新数据就不用再翻页
        if (list.length < pageSize) break
      }
      const all = [...byId.values()]
      // 兼容层：服务端万一没筛（老版本），这里再按「没有头像」算一次，
      // 保证「批量匹配无头像」永远不会去动已经有头像的艺人。
      setResults(noImageOnly ? all.filter((a) => !hasAvatar(a)) : all)
    } catch (e) {
      console.error('Failed to load artists:', e)
      notify('加载艺人失败', 'warning')
    } finally {
      setLoading(false)
    }
  }

  // 检查艺人名字是否有效（过滤掉包含歌词、时间戳等污染数据）
  const isValidArtistName = (name) => {
    if (!name || name.length < 1) return false
    // 过滤包含LRC时间戳格式 [00:00.000]
    if (/\[\d{2}:\d{2}[.:]\d{2,3}\]/.test(name)) return false
    // 过滤包含LRC标签 [ver:...] [ti:...] [ar:...]
    if (/\[(ver|ti|ar|al|by):/i.test(name)) return false
    // 过滤包含歌词制作人标记
    if (/[词曲编制作人演唱者：]/.test(name) && name.length > 20) return false
    // 过滤包含大量中文歌词内容（超过50个中文字符且没有分隔符）
    const chineseChars = (name.match(/[\u4e00-\u9fff]/g) || []).length
    if (chineseChars > 30 && !name.includes('/') && !name.includes('、')) return false
    // 过滤包含英文歌词（连续英文单词超过10个）
    if (/\b(\w+\s+){10,}\w+\b/.test(name)) return false
    return true
  }

  const handleBatchSave = async () => {
    if (!results || results.length === 0) return
    
    // 过滤掉名字无效的艺人
    const validArtists = results.filter(a => isValidArtistName(a.name))
    const skippedCount = results.length - validArtists.length
    
    if (validArtists.length === 0) {
      notify('没有有效的艺人可以匹配', 'warning')
      return
    }
    
    setSaving(true)
    setProgress({ current: 0, total: validArtists.length })

    let successCount = 0
    let failCount = 0

    for (let i = 0; i < validArtists.length; i++) {
      const artist = validArtists[i]
      setProgress({ current: i + 1, total: validArtists.length })

      try {
        // Search for avatar
        const searchRes = await httpClient(`${REST_URL}/scrape/artist?q=${encodeURIComponent(artist.name)}`)
        const searchResults = searchRes.json?.data || []

        if (searchResults.length === 0) {
          failCount++
          continue
        }

        // Find best match: prefer exact name match, then highest quality source
        const artistNameLower = artist.name.toLowerCase().trim()
        let avatar = searchResults[0] // default to first result
        for (const r of searchResults) {
          const rNameLower = (r.name || '').toLowerCase().trim()
          if (rNameLower === artistNameLower) {
            avatar = r
            break
          }
        }
        // Skip if top result name is completely unrelated (no common substring > 1 char)
        const topName = (avatar.name || '').toLowerCase().trim()
        const hasCommon = topName.includes(artistNameLower) || artistNameLower.includes(topName) ||
          (topName.length > 1 && artistNameLower.length > 1 && (
            topName.substring(0, 2) === artistNameLower.substring(0, 2)
          ))
        if (!hasCommon && topName !== artistNameLower) {
          failCount++
          continue
        }
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

      // Delay to avoid overwhelming the server and ensure files are written
      await new Promise(resolve => setTimeout(resolve, 1000))
    }

    setSaving(false)
    const skippedMsg = skippedCount > 0 ? `，跳过${skippedCount}个无效艺人` : ''
    notify(`完成: ${successCount} 成功, ${failCount} 失败${skippedMsg}`, successCount > 0 ? 'info' : 'warning')
    // Reload page to show new avatars (with cache-busting)，保留原有查询参数（含筛选条件）
    if (successCount > 0) {
      setTimeout(() => {
        const search = new URLSearchParams(window.location.search)
        search.set('t', String(Date.now()))
        window.location.search = search.toString()
      }, 2000)
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
        <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>
          {noImageOnly ? '🔍 批量匹配无头像艺人' : '🔍 批量匹配艺人头像'}
        </h2>

        <p style={{ fontSize: 13, color: '#666', marginBottom: 16 }}>
          {noImageOnly
            ? '只处理当前筛选出的「未设置头像」艺人。会从网易云音乐、QQ音乐、酷我音乐、酷狗音乐搜索匹配的头像图片。'
            : '自动为所有艺人搜索并保存头像。会从网易云音乐、QQ音乐、酷我音乐、酷狗音乐搜索匹配的头像图片。'}
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
            <p style={{ fontWeight: 600 }}>
              共 {results.length} 位{noImageOnly ? '无头像' : ''}艺人
            </p>
            <div style={{ maxHeight: 300, overflow: 'auto', marginBottom: 16 }}>
              {results.map((artist, idx) => {
                const valid = isValidArtistName(artist.name)
                return (
                  <div key={artist.id} style={{
                    padding: '6px 12px', borderBottom: '1px solid #eee', fontSize: 13,
                    color: valid ? '#333' : '#999',
                    textDecoration: valid ? 'none' : 'line-through',
                  }}>
                    {idx + 1}. {artist.name.substring(0, 60)}{artist.name.length > 60 ? '...' : ''}
                    {!valid && <span style={{ fontSize: 11, color: '#f44336', marginLeft: 8 }}>(将跳过)</span>}
                  </div>
                )
              })}
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
