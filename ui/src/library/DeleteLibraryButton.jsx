import React, { useState } from 'react'
import DeleteIcon from '@material-ui/icons/Delete'
import { makeStyles, alpha } from '@material-ui/core/styles'
import clsx from 'clsx'
import {
  useNotify,
  Button,
  Confirm,
  useTranslate,
  useRedirect,
  useRefresh,
} from 'react-admin'
import { REST_URL } from '../consts'

const useStyles = makeStyles(
  (theme) => ({
    deleteButton: {
      color: theme.palette.error.main,
      '&:hover': {
        backgroundColor: alpha(theme.palette.error.main, 0.12),
        // Reset on mouse devices
        '@media (hover: none)': {
          backgroundColor: 'transparent',
        },
      },
    },
  }),
  { name: 'RaDeleteWithConfirmButton' },
)

const authHeaders = () => ({
  'X-ND-Authorization': `Bearer ${localStorage.getItem('token')}`,
})

// Self-contained delete (plain fetch instead of useDeleteWithConfirmController):
// the API endpoint DELETE /api/library/{id} is straightforward and this keeps the
// button working regardless of react-admin controller internals.
const DeleteLibraryButton = ({ record, className, ...props }) => {
  const translate = useTranslate()
  const notify = useNotify()
  const redirect = useRedirect()
  const refresh = useRefresh()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleDelete = async () => {
    if (!record || record.id === undefined) {
      setOpen(false)
      return
    }
    setLoading(true)
    try {
      const res = await fetch(`${REST_URL}/library/${record.id}`, {
        method: 'DELETE',
        headers: authHeaders(),
      })
      if (!res.ok) {
        const body = await res.text().catch(() => '')
        throw new Error(body || `HTTP ${res.status}`)
      }
      setOpen(false)
      setLoading(false)
      notify('resources.library.notifications.deleted', 'info', {
        smart_count: 1,
      })
      refresh()
      redirect('/library')
    } catch (e) {
      setLoading(false)
      notify(e.message || 'ra.notification.http_error', 'warning')
    }
  }

  const classes = useStyles(props)
  return (
    <>
      <Button
        label="ra.action.delete"
        onClick={(e) => {
          setOpen(true)
          e.stopPropagation()
        }}
        disabled={loading}
        className={clsx('ra-delete-button', classes.deleteButton, className)}
        {...props}
      >
        <DeleteIcon />
      </Button>
      <Confirm
        isOpen={open}
        loading={loading}
        title={translate('resources.library.name', { smart_count: 1 })}
        content={translate('resources.library.messages.deleteConfirm')}
        onConfirm={handleDelete}
        onClose={() => setOpen(false)}
      />
    </>
  )
}

export default DeleteLibraryButton
