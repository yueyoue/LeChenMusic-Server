import AudiobookList from './AudiobookList'
import AudiobookDetail from './AudiobookDetail'
import AudiobookPlayer from './AudiobookPlayer'
import AudiobookPlayerContainer from './AudiobookPlayerContainer'
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

export { AudiobookDetail, AudiobookPlayer, AudiobookPlayerContainer, NarratorList, NarratorDetail }
export default { all, admin }
