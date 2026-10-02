import React from 'react'
import { Button, ButtonGroup, makeStyles } from '@material-ui/core'
import ViewHeadlineIcon from '@material-ui/icons/ViewHeadline'
import ViewModuleIcon from '@material-ui/icons/ViewModule'
import { useDispatch, useSelector } from 'react-redux'
import { audiobookViewGrid, audiobookViewTable } from '../actions'

const useStyles = makeStyles({
  buttonGroup: { justifyContent: 'center' },
  leftButton: { paddingRight: '0.5rem' },
  rightButton: { paddingLeft: '0.5rem' },
})

/**
 * 有声书布局切换（网格 / 表格）——与「专辑 → 全部」右上角的切换一致。
 * 状态存在 redux 的 audiobookView 里（同 albumView），所有分类页共享同一布局。
 */
const AudiobookViewToggler = () => {
  const classes = useStyles()
  const dispatch = useDispatch()
  const grid = useSelector((state) => state.audiobookView?.grid !== false)

  return (
    <ButtonGroup
      variant="text"
      color="primary"
      aria-label="有声书布局切换"
      className={classes.buttonGroup}
    >
      <Button
        size="small"
        className={classes.leftButton}
        aria-label="网格显示"
        color={grid ? 'primary' : 'secondary'}
        onClick={() => dispatch(audiobookViewGrid())}
      >
        <ViewModuleIcon fontSize="inherit" />
      </Button>
      <Button
        size="small"
        className={classes.rightButton}
        aria-label="表格显示"
        color={grid ? 'secondary' : 'primary'}
        onClick={() => dispatch(audiobookViewTable())}
      >
        <ViewHeadlineIcon fontSize="inherit" />
      </Button>
    </ButtonGroup>
  )
}

export default AudiobookViewToggler
