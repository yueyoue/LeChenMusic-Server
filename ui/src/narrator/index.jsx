import NarratorList from '../audiobook/NarratorList'
import React from 'react'

const NarratorIcon = () => <span style={{ fontSize: 20 }}>🎤</span>

const all = {
  list: NarratorList,
  icon: <NarratorIcon />,
}

const admin = {
  ...all,
}

export default { all, admin }
