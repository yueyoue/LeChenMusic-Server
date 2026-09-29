import PropTypes from 'prop-types'
import { Tooltip } from '@material-ui/core'
import { useTranslate } from 'react-admin'

// 统一的「失效条目 hover 提示」（评审 §4.6 P2-2）。
//
// 各列表对 missing 条目的灰置样式（opacity）保留原样，本组件只负责"为什么灰"的
// hover 说明：歌曲列表/专辑曲目/歌单曲目/收藏/专辑视图/艺人列表等所有出现媒体条目的
// 场景共用同一份文案与交互，避免各处各写一套。
//
// 用法（包在行/卡片元素外层即可，非 missing 时原样返回 children，零行为变化）：
//   <MissingHint record={record}>
//     <DatagridRow ... />
//   </MissingHint>
export const MissingHint = ({ record, children, placement = 'top', ...rest }) => {
  const translate = useTranslate()
  if (!record?.missing) {
    return children
  }
  return (
    <Tooltip
      title={translate('message.missingFileHint')}
      placement={placement}
      arrow
      {...rest}
    >
      {children}
    </Tooltip>
  )
}

MissingHint.propTypes = {
  record: PropTypes.object,
  children: PropTypes.node,
  placement: PropTypes.string,
}
