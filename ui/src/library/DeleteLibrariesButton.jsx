import React, { useState } from 'react'
import DeleteIcon from '@material-ui/icons/Delete'
import {
  Button,
  Confirm,
  useListContext,
  useNotify,
  useRefresh,
  useTranslate,
} from 'react-admin'
import { REST_URL } from '../consts'

const authHeaders = () => ({
  'X-ND-Authorization': `Bearer ${localStorage.getItem('token')}`,
})

// Bulk delete for the media-library list (the only delete entry point before was
// the edit-page button). Library 1 (main library) is protected and skipped.
const DeleteLibrariesButton = () => {
  const translate = useTranslate()
  const notify = useNotify()
  const refresh = useRefresh()
  const { selectedIds, onUnselectItems } = useListContext()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleDelete = async () => {
    setLoading(true)
    const ids = (selectedIds || []).filter((id) => String(id) !== '1')
    let failed = 0
    for (const id of ids) {
      try {
        const res = await fetch(`${REST_URL}/library/${id}`, {
          method: 'DELETE',
          headers: authHeaders(),
        })
        if (!res.ok) failed++
      } catch (e) {
        failed++
      }
    }
    setOpen(false)
    setLoading(false)
    if (failed > 0) {
      notify('resources.library.notifications.deleteError', 'warning', {
        failed,
      })
    }
    notify('resources.library.notifications.deleted', 'info', {
      smart_count: ids.length,
    })
    onUnselectItems()
    refresh()
  }

  return (
    <>
      <Button
        label="ra.action.delete"
        onClick={(e) => {
          setOpen(true)
          e.stopPropagation()
        }}
        disabled={loading}
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={loading}
        title={translate('resources.library.name', { smart_count: 2 })}
        content={translate('resources.library.messages.deleteConfirmBulk', {
          count: (selectedIds || []).filter((id) => String(id) !== '1').length,
        })}
        onConfirm={handleDelete}
        onClose={() => setOpen(false)}
      />
    </>
  )
}

export default DeleteLibrariesButton
