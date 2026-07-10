import React from 'react'
import { Route } from 'react-router-dom'
import Personal from './personal/Personal'
import AudiobookList from './audiobook/AudiobookList'
import AudiobookDetail from './audiobook/AudiobookDetail'
import NarratorList from './audiobook/NarratorList'
import VersionPage from './settings/VersionPage'
import AppManagePage from './appmanage/AppManagePage'
import FavoritesPage from './favorites/FavoritesPage'
import AIPlaylistPage from './ai-playlist/AIPlaylistPage'

const routes = [
  <Route exact path="/personal" render={() => <Personal />} key={'personal'} />,
  <Route exact path="/favorites" render={() => <FavoritesPage />} key={'favorites'} />,
  <Route exact path="/ai-playlist" render={() => <AIPlaylistPage />} key={'ai-playlist'} />,
  <Route exact path="/audiobook" render={() => <AudiobookList />} key={'audiobook'} />,
  <Route exact path="/audiobook/starred" render={() => <AudiobookList />} key={'audiobook-starred'} />,
  <Route exact path="/audiobook/:id" render={({ match }) => <AudiobookDetail id={match.params.id} />} key={'audiobook-detail'} />,
  <Route exact path="/narrator" render={() => <NarratorList />} key={'narrator'} />,
  <Route exact path="/settings/version" render={() => <VersionPage />} key={'version'} />,
  <Route exact path="/settings/app" render={() => <AppManagePage />} key={'app-manage'} />,
]

export default routes
