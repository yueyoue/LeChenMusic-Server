import React, { useState } from 'react'
import PropTypes from 'prop-types'
import { useDispatch } from 'react-redux'
import { useMediaQuery, CircularProgress, IconButton, Tooltip } from '@material-ui/core'
import { makeStyles } from '@material-ui/core/styles'
import {
  Button,
  useDataProvider,
  useNotify,
  useTranslate,
} from 'react-admin'
import ShuffleIcon from '@material-ui/icons/Shuffle'
import PlayArrowIcon from '@material-ui/icons/PlayArrow'
import SearchIcon from '@material-ui/icons/Search'
import { IoIosRadio } from 'react-icons/io'
import { playShuffle, playTopSongs } from './actions.js'
import { playSimilar } from '../common/playbackActions.js'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const useStyles = makeStyles((theme) => ({
  root: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    flexWrap: 'wrap',
  },
  button: {
    [theme.breakpoints.down('xs')]: {
      minWidth: 'auto',
      padding: '8px 12px',
      fontSize: '0.75rem',
      '& .MuiButton-startIcon': {
        marginRight: '4px',
      },
    },
  },
  radioIcon: {
    [theme.breakpoints.down('xs')]: {
      fontSize: '1.5rem',
    },
  },
  scrapeBtn: {
    marginLeft: 8,
    border: '1px dashed rgba(255,255,255,0.3)',
    borderRadius: 4,
    padding: '4px 12px',
    fontSize: 13,
    cursor: 'pointer',
    '&:hover': {
      backgroundColor: 'rgba(255,255,255,0.1)',
    },
  },
  dialogOverlay: {
    position: 'fixed',
    top: 0, left: 0, right: 0, bottom: 0,
    backgroundColor: 'rgba(0,0,0,0.5)',
    zIndex: 9999,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  dialog: {
    backgroundColor: '#fff',
    borderRadius: 8,
    width: '90%',
    maxWidth: 500,
    maxHeight: '80vh',
    overflow: 'auto',
    padding: 24,
    color: '#333',
  },
  dialogTitle: {
    fontSize: 18,
    fontWeight: 700,
    marginBottom: 16,
  },
  resultItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 12,
    padding: '8px 12px',
    border: '1px solid #e0e0e0',
    borderRadius: 8,
    marginBottom: 8,
    cursor: 'pointer',
    '&:hover': { backgroundColor: '#f5f5f5' },
  },
  resultItemSelected: {
    border: '2px solid #1976d2',
  },
  resultImg: {
    width: 48, height: 48, borderRadius: '50%', objectFit: 'cover',
  },
  resultInfo: {
    flex: 1,
  },
  resultName: { fontSize: 14, fontWeight: 600 },
  resultPlatform: { fontSize: 11, color: '#666' },
  urlInput: {
    width: '100%',
    padding: '8px 12px',
    border: '1px solid #ccc',
    borderRadius: 4,
    fontSize: 14,
    marginTop: 8,
  },
  previewImg: {
    maxWidth: 200, maxHeight: 200, borderRadius: 8, marginTop: 8,
  },
  actions: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: 8,
    marginTop: 16,
  },
  saveBtn: {
    backgroundColor: '#1976d2',
    color: '#fff',
    border: 'none',
    borderRadius: 4,
    padding: '8px 24px',
    fontSize: 14,
    cursor: 'pointer',
    '&:disabled': { opacity: 0.5 },
  },
  cancelBtn: {
    backgroundColor: '#e0e0e0',
    border: 'none',
    borderRadius: 4,
    padding: '8px 24px',
    fontSize: 14,
    cursor: 'pointer',
  },
}))

const ArtistAvatarDialog = ({ open, onClose, artist, onApply }) => {
  const classes = useStyles()
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
      const res = await httpClient(`${REST_URL}/scrape/artist?q=${encodeURIComponent(query)}`)
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
      console.log('Saving avatar:', { artistId: artist.id, imageUrl })
      const res = await httpClient(`${REST_URL}/scrape/artist/${encodeURIComponent(artist.id)}/avatar`, {
        method: 'POST',
        body: JSON.stringify({ imageUrl }),
      })
      console.log('Avatar save response:', JSON.stringify(res.json))
      const savedUrl = res.json?.data?.imageUrl
      if (!savedUrl) {
        console.error('No imageUrl in response:', res.json)
        alert('保存失败: 服务器未返回图片URL')
        return
      }
      console.log('Avatar saved successfully:', savedUrl)
      if (onApply) onApply()
      onClose()
    } catch (e) {
      console.error('Avatar save error:', e)
      alert('保存失败: ' + e.message)
    } finally {
      setApplying(false)
    }
  }

  if (!open) return null

  return (
    <div className={classes.dialogOverlay} onClick={onClose}>
      <div className={classes.dialog} onClick={e => e.stopPropagation()}>
        <div className={classes.dialogTitle}>🔍 匹配头像</div>

        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input
            className={classes.urlInput}
            style={{ marginTop: 0, flex: 1 }}
            placeholder="输入名称搜索"
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyPress={e => e.key === 'Enter' && handleSearch()}
          />
          <button className={classes.saveBtn} onClick={handleSearch} disabled={loading}>
            {loading ? '...' : '搜索'}
          </button>
        </div>

        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <button className={!urlMode ? classes.saveBtn : classes.cancelBtn}
            onClick={() => setUrlMode(false)}>搜索结果</button>
          <button className={urlMode ? classes.saveBtn : classes.cancelBtn}
            onClick={() => setUrlMode(true)}>输入URL</button>
        </div>

        {urlMode ? (
          <div>
            <input className={classes.urlInput} placeholder="https://example.com/avatar.jpg"
              value={manualUrl} onChange={e => setManualUrl(e.target.value)} />
            {manualUrl && <img src={manualUrl} className={classes.previewImg} alt="预览"
              onError={e => e.target.style.display = 'none'} />}
          </div>
        ) : (
          <div>
            {results.length === 0 && !loading && (
              <div style={{ textAlign: 'center', color: '#999', padding: 20 }}>输入名称搜索头像</div>
            )}
            {results.map((item, idx) => (
              <div key={idx}
                className={`${classes.resultItem} ${selected?.imageUrl === item.imageUrl ? classes.resultItemSelected : ''}`}
                onClick={() => setSelected(item)}>
                <input type="radio" checked={selected?.imageUrl === item.imageUrl} readOnly />
                <img src={item.imageUrl} className={classes.resultImg} alt="" />
                <div className={classes.resultInfo}>
                  <div className={classes.resultName}>{item.name}</div>
                  <div className={classes.resultPlatform}>来源: {item.platform}</div>
                </div>
              </div>
            ))}
          </div>
        )}

        <div className={classes.actions}>
          <button className={classes.cancelBtn} onClick={onClose}>取消</button>
          <button className={classes.saveBtn} onClick={handleApply}
            disabled={applying || (!urlMode && !selected) || (urlMode && !manualUrl.trim())}>
            {applying ? '保存中...' : '保存头像'}
          </button>
        </div>
      </div>
    </div>
  )
}

const LoadingButton = ({ loading, icon, ...rest }) => (
  <Button {...rest}>
    {loading ? <CircularProgress size={20} color="inherit" /> : icon}
  </Button>
)

const ArtistActions = ({ className, record, ...rest }) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const dataProvider = useDataProvider()
  const notify = useNotify()
  const classes = useStyles()
  const isMobile = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  const [loadingAction, setLoadingAction] = useState(null)
  const isLoading = !!loadingAction
  const [dialogOpen, setDialogOpen] = useState(false)

  const handlePlay = React.useCallback(async () => {
    setLoadingAction('play')
    try {
      await playTopSongs(dispatch, notify, record.name)
    } catch (e) {
      console.error('Error fetching top songs for artist:', e)
      notify('ra.page.error', 'warning')
    } finally {
      setLoadingAction(null)
    }
  }, [dispatch, notify, record])

  const handleShuffle = React.useCallback(async () => {
    setLoadingAction('shuffle')
    try {
      await playShuffle(dataProvider, dispatch, record.id)
    } catch (e) {
      console.error('Error fetching songs for shuffle:', e)
      notify('ra.page.error', 'warning')
    } finally {
      setLoadingAction(null)
    }
  }, [dataProvider, dispatch, record, notify])

  const handleRadio = React.useCallback(async () => {
    setLoadingAction('radio')
    try {
      await playSimilar(dispatch, notify, record.id)
    } catch (e) {
      console.error('Error starting radio for artist:', e)
      notify('ra.page.error', 'warning')
    } finally {
      setLoadingAction(null)
    }
  }, [dispatch, notify, record])

  return (
    <div className={`${classes.root} ${className || ''}`}>
      <LoadingButton
        onClick={handlePlay}
        label={translate('resources.artist.actions.topSongs')}
        className={classes.button}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'play'}
        icon={<PlayArrowIcon />}
      />
      <LoadingButton
        onClick={handleShuffle}
        label={translate('resources.artist.actions.shuffle')}
        className={classes.button}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'shuffle'}
        icon={<ShuffleIcon />}
      />
      <LoadingButton
        onClick={handleRadio}
        label={translate('resources.artist.actions.radio')}
        className={classes.button}
        size={isMobile ? 'small' : 'medium'}
        disabled={isLoading}
        loading={loadingAction === 'radio'}
        icon={<IoIosRadio className={classes.radioIcon} />}
      />
      <Tooltip title="搜索匹配艺人头像">
        <span className={classes.scrapeBtn} onClick={() => setDialogOpen(true)}>
          🔍 匹配头像
        </span>
      </Tooltip>
      <ArtistAvatarDialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        artist={record}
        onApply={() => window.location.reload()}
      />
    </div>
  )
}

ArtistActions.propTypes = {
  className: PropTypes.string,
  record: PropTypes.object.isRequired,
}

ArtistActions.defaultProps = {
  className: '',
}

export default ArtistActions
