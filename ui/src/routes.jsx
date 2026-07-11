import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudiobookList from './audiobook/AudiobookList'
import AudiobookDetail from './audiobook/AudiobookDetail'
import NarratorList from './audiobook/NarratorList'
import NarratorDetail from './audiobook/NarratorDetail'
import VersionPage from './settings/VersionPage'
import AppManagePage from './appmanage/AppManagePage'
import FavoritesPage from './favorites/FavoritesPage'
import AIPlaylistPage from './ai-playlist/AIPlaylistPage'
import ErrorLogPage from './error-log/ErrorLogPage'
import BackupPage from './backup/BackupList'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/favorites" render={() => <FavoritesPage />} key={'favorites'} />,
  <Route exact path="/ai-playlist" render={() => <AIPlaylistPage />} key={'ai-playlist'} />,
  <Route exact path="/error-log" render={() => <ErrorLogPage />} key={'error-log'} />,
  <Route exact path="/audiobook" render={() => <AudiobookList />} key={'audiobook'} />,
  <Route exact path="/audiobook/starred" render={() => <AudiobookList />} key={'audiobook-starred'} />,
  <Route exact path="/audiobook/:id" render={({ match }) => <AudiobookDetail id={match.params.id} />} key={'audiobook-detail'} />,
  <Route exact path="/narrator" render={() => <NarratorList />} key={'narrator'} />,
  <Route exact path="/narrator/:name" render={({ match }) => <NarratorDetail name={decodeURIComponent(match.params.name)} onBack={() => window.history.back()} />} key={'narrator-detail'} />,
  <Route exact path="/settings/version" render={() => <VersionPage />} key={'version'} />,
  <Route exact path="/settings/app" render={() => <AppManagePage />} key={'app-manage'} />,
  <Route exact path="/settings/backup" render={() => <BackupPage />} key={'backup'} />,
]

export default routes
