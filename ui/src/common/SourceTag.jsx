import { Chip } from '@material-ui/core'
import { useTranslate } from 'react-admin'
import { isCloudPath } from '../library/cloudPath'

// 统一的「本地 / 网盘」来源标识（评审 §4.6 P2-3）。
//
// 歌曲列表、专辑/歌单曲目、有声书卡片等所有出现媒体条目的地方共用这一个组件，
// 保证文案、配色、样式全局一致；本地与云库的判定口径是 isCloudPath()
// （即库路径是否为 openlist://，与后端 core/cloudsource 的 BuildURI 对齐）。
//
// 用法（react-admin 字段里包一层 FunctionField，或直接在自定义布局里用）：
//   <FunctionField source="libraryPath" render={(r) => <SourceTag libraryPath={r?.libraryPath} />} />
export const SourceTag = ({ libraryPath, size = 'small', style, ...rest }) => {
  const translate = useTranslate()
  const isCloud = isCloudPath(libraryPath)
  return (
    <Chip
      size={size}
      variant="outlined"
      label={
        isCloud
          ? translate('resources.library.cloud.tagCloud')
          : translate('resources.library.cloud.tagLocal')
      }
      style={{
        ...(isCloud
          ? { borderColor: '#1976d2', color: '#1976d2' }
          : { borderColor: '#9e9e9e', color: '#616161' }),
        ...style,
      }}
      {...rest}
    />
  )
}
