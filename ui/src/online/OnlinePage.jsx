import React, { useState } from 'react'
import { Typography, Box, makeStyles, Tabs, Tab, Badge } from '@material-ui/core'
import MusicNoteIcon from '@material-ui/icons/MusicNote'
import SearchIcon from '@material-ui/icons/Search'
import GetAppIcon from '@material-ui/icons/GetApp'
import StorageIcon from '@material-ui/icons/Storage'
import SettingsIcon from '@material-ui/icons/Settings'
import HistoryIcon from '@material-ui/icons/History'
import OnlineSearchSection from './OnlineSearchSection'
import OnlineTasksSection from './OnlineTasksSection'
import OnlineDownloadsSection from './OnlineDownloadsSection'
import OnlineSettingsSection from './OnlineSettingsSection'
import OnlineLogsSection from './OnlineLogsSection'

// 「在线搜歌」单页整合（M5.1）：
//   🔍 搜歌下载 → 📥 下载任务 → 📦 下载管理(admin) → ⚙️ 权限设置(admin) → 🕘 操作日志(admin)
// 功能区合并为一个页面、一个菜单入口，通过顶部标签切换。

const useStyles = makeStyles((theme) => ({
  root: { padding: 16, maxWidth: 1200, margin: '0 auto' },
  header: { display: 'flex', alignItems: 'center', gap: 12, marginBottom: 4 },
  tabsBar: { marginBottom: 16, borderBottom: `1px solid ${theme.palette.divider}` },
}))

const OnlinePage = ({ defaultTab = 'search' }) => {
  const classes = useStyles()
  const isAdmin = typeof window !== 'undefined' && localStorage.getItem('role') === 'admin'
  const [tab, setTab] = useState(defaultTab)
  const [activeCount, setActiveCount] = useState(0)

  return (
    <Box className={classes.root}>
      <Box className={classes.header}>
        <MusicNoteIcon style={{ fontSize: 28, color: '#1e90ff' }} />
        <Typography variant="h6" style={{ fontWeight: 700 }}>在线搜歌</Typography>
        <Typography style={{ fontSize: 12, color: '#888' }}>
          搜不到的歌，一键下载入库 · 自动刮削封面/歌词/标签
        </Typography>
      </Box>

      <Tabs
        className={classes.tabsBar}
        value={tab}
        onChange={(_, v) => setTab(v)}
        variant="scrollable"
        scrollButtons="auto"
        indicatorColor="primary"
        textColor="primary"
      >
        <Tab value="search" icon={<SearchIcon />} label="搜歌下载" />
        <Tab
          value="tasks"
          icon={
            <Badge badgeContent={activeCount} color="secondary" max={99}>
              <GetAppIcon />
            </Badge>
          }
          label="下载任务"
        />
        {isAdmin && <Tab value="downloads" icon={<StorageIcon />} label="下载管理" />}
        {isAdmin && <Tab value="settings" icon={<SettingsIcon />} label="权限设置" />}
        {isAdmin && <Tab value="logs" icon={<HistoryIcon />} label="操作日志" />}
      </Tabs>

      {tab === 'search' && <OnlineSearchSection onTasksChanged={() => setTab('tasks')} />}
      {tab === 'tasks' && <OnlineTasksSection onActiveCount={setActiveCount} />}
      {tab === 'downloads' && isAdmin && <OnlineDownloadsSection />}
      {tab === 'settings' && isAdmin && <OnlineSettingsSection />}
      {tab === 'logs' && isAdmin && <OnlineLogsSection />}
    </Box>
  )
}

export default OnlinePage
