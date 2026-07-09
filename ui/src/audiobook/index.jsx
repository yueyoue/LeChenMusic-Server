import AudiobookList from './AudiobookList'
import AudiobookDetail from './AudiobookDetail'
import AudiobookPlayer from './AudiobookPlayer'
import NarratorList from './NarratorList'
import NarratorDetail from './NarratorDetail'
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

export { AudiobookDetail, AudiobookPlayer, NarratorList, NarratorDetail }
export default { all, admin }
