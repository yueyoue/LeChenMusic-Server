import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudiobookList from './audiobook/AudiobookList'
import AudiobookDetail from './audiobook/AudiobookDetail'
import NarratorList from './audiobook/NarratorList'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/audiobook" render={() => <AudiobookList />} key={'audiobook'} />,
  <Route exact path="/audiobook/starred" render={() => <AudiobookList />} key={'audiobook-starred'} />,
  <Route exact path="/audiobook/:id" render={({ match }) => <AudiobookDetail id={match.params.id} />} key={'audiobook-detail'} />,
  <Route exact path="/narrator" render={() => <NarratorList />} key={'narrator'} />,
]

export default routes
