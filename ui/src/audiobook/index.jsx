import AudiobookList from './AudiobookList'
import React from 'react'

const AudiobookIcon = () => <span style={{ fontSize: 20 }}>📖</span>
const AudiobookOutlinedIcon = () => <span style={{ fontSize: 20 }}>📖</span>

const all = {
  list: AudiobookList,
  icon: <AudiobookOutlinedIcon />,
}

const admin = {
  ...all,
}

export default { all, admin }
