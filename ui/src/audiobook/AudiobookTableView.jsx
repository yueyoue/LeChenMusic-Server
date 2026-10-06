import React from 'react'
import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
  makeStyles,
} from '@material-ui/core'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import { OverflowTooltip, SourceTag, CoverImage } from '../common'

const useStyles = makeStyles((theme) => ({
  row: {
    cursor: 'pointer',
    '&:hover': { backgroundColor: theme.palette.action.hover },
  },
  titleCell: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    minWidth: 220,
  },
  thumb: {
    width: 36,
    height: 36,
    objectFit: 'cover',
    borderRadius: 4,
    flexShrink: 0,
    backgroundColor: theme.palette.grey[200],
  },
  thumbPlaceholder: {
    width: 36,
    height: 36,
    borderRadius: 4,
    flexShrink: 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
  },
  muted: { color: theme.palette.text.secondary },
}))

const coverUrl = (book) =>
  `/api/audiobook/${book.id}/cover?token=${localStorage.getItem('token') || ''}`

const formatDuration = (seconds) => {
  if (!seconds || seconds <= 0) return ''
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return h > 0 ? `${h}小时${m}分` : `${m}分`
}

const Thumb = ({ book }) => {
  const classes = useStyles()
  return (
    <CoverImage
      src={coverUrl(book)}
      alt=""
      className={classes.thumb}
      fallback={
        <div className={classes.thumbPlaceholder}>
          <MenuBookIcon style={{ fontSize: 18, opacity: 0.6, color: '#fff' }} />
        </div>
      }
    />
  )
}

/**
 * 有声书表格视图——与「专辑 → 全部」的表格布局对应。
 * 列：书名 / 作者 / 演播者 / 分类 / 章节 / 时长 / 来源（W=网盘，B=本地）
 */
const AudiobookTableView = ({ books, onOpen }) => {
  const classes = useStyles()

  return (
    <TableContainer component={Paper} elevation={0}>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>书名</TableCell>
            <TableCell>作者</TableCell>
            <TableCell>演播者</TableCell>
            <TableCell>分类</TableCell>
            <TableCell align="right">章节</TableCell>
            <TableCell align="right">时长</TableCell>
            <TableCell>来源</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {books.map((book) => (
            <TableRow
              key={book.id}
              hover
              className={classes.row}
              onClick={() => onOpen(book)}
            >
              <TableCell>
                <div className={classes.titleCell}>
                  <Thumb book={book} />
                  <OverflowTooltip title={book.title}>
                    <Typography variant="body2" noWrap style={{ maxWidth: 280 }}>
                      {book.title}
                    </Typography>
                  </OverflowTooltip>
                </div>
              </TableCell>
              <TableCell className={classes.muted}>{book.author || '—'}</TableCell>
              <TableCell className={classes.muted}>{book.narrator || '—'}</TableCell>
              <TableCell className={classes.muted}>{book.genre || '—'}</TableCell>
              <TableCell align="right" className={classes.muted}>
                {book.chapterCount > 0 ? `${book.chapterCount}章` : '—'}
              </TableCell>
              <TableCell align="right" className={classes.muted}>
                {formatDuration(book.totalDuration) || '—'}
              </TableCell>
              <TableCell>
                <SourceTag libraryPath={book?.libraryPath} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

export default AudiobookTableView
