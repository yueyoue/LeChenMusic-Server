import React, { cloneElement, useState } from 'react'
import { sanitizeListRestProps, TopToolbar } from 'react-admin'
import { useMediaQuery, Button, Tooltip } from '@material-ui/core'
import FileCopyIcon from '@material-ui/icons/FileCopy'
import { ShuffleAllButton, ToggleFieldsMenu } from '../common'
import DuplicateSongsDialog from '../dialogs/DuplicateSongsDialog'

export const SongListActions = ({
  currentSort,
  className,
  resource,
  filters,
  displayedFilters,
  filterValues,
  permanentFilter,
  exporter,
  basePath,
  selectedIds,
  onUnselectItems,
  showFilter,
  maxResults,
  total,
  ids,
  ...rest
}) => {
  const isNotSmall = useMediaQuery((theme) => theme.breakpoints.up('sm'))
  const [dupOpen, setDupOpen] = useState(false)
  return (
    <TopToolbar className={className} {...sanitizeListRestProps(rest)}>
      <ShuffleAllButton filters={filterValues} />
      <Tooltip title="检测重复歌曲">
        <Button
          size="small"
          startIcon={<FileCopyIcon />}
          onClick={() => setDupOpen(true)}
        >
          重复检测
        </Button>
      </Tooltip>
      {filters &&
        cloneElement(filters, {
          resource,
          showFilter,
          displayedFilters,
          filterValues,
          context: 'button',
        })}
      {isNotSmall && <ToggleFieldsMenu resource="song" />}
      <DuplicateSongsDialog open={dupOpen} onClose={() => setDupOpen(false)} />
    </TopToolbar>
  )
}

SongListActions.defaultProps = {
  selectedIds: [],
  onUnselectItems: () => null,
}
