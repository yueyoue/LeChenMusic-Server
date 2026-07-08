import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudiobookList from './audiobook/AudiobookList'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/audiobook/starred" render={() => <AudiobookList />} key={'audiobook-starred'} />,
  <Route exact path="/narrator" render={() => <div style={{padding: 20}}><h2>🎤 演播者</h2><p>演播者页面开发中...</p></div>} key={'narrator'} />,
]

export default routes
