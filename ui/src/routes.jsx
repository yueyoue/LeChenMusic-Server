import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudiobookList from './audiobook/AudiobookList'
import AudiobookDetail from './audiobook/AudiobookDetail'
import NarratorList from './audiobook/NarratorList'
import VersionPage from './settings/VersionPage'
import AppManagePage from './appmanage/AppManagePage'

const AudiobookDetailRoute = ({ match }) => {
  const id = match.params.id
  const handleBack = () => { window.location.hash = '#/audiobook' }
  const handlePlay = (book, chapters, chapter) => {
    // Dispatch custom event for the audiobook player to pick up
    window.dispatchEvent(new CustomEvent('audiobook-play', {
      detail: { book, chapters, chapter }
    }))
  }
  return <AudiobookDetail id={id} onBack={handleBack} onPlay={handlePlay} />
}

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/audiobook" render={() => <AudiobookList />} key={'audiobook'} />,
  <Route exact path="/audiobook/starred" render={() => <AudiobookList />} key={'audiobook-starred'} />,
  <Route exact path="/audiobook/:id" render={(props) => <AudiobookDetailRoute {...props} />} key={'audiobook-detail'} />,
  <Route exact path="/narrator" render={() => <NarratorList />} key={'narrator'} />,
  <Route exact path="/settings/version" render={() => <VersionPage />} key={'version'} />,
  <Route exact path="/settings/app" render={() => <AppManagePage />} key={'app-manage'} />,
]

export default routes
