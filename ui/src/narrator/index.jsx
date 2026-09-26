import NarratorList from '../audiobook/NarratorList'
import React from 'react'

const all = {
  list: NarratorList,
  icon: <span style={{ fontSize: 20 }}>🎤</span>,
}

const admin = {
  ...all,
}

export default { all, admin }
