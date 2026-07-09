import React, { useState } from 'react'
import { useSelector } from 'react-redux'
import { Divider, makeStyles } from '@material-ui/core'
import clsx from 'clsx'
import { useTranslate, MenuItemLink, getResources } from 'react-admin'
import ViewListIcon from '@material-ui/icons/ViewList'
import AlbumIcon from '@material-ui/icons/Album'
import MenuBookIcon from '@material-ui/icons/MenuBook'
import SystemUpdateIcon from '@material-ui/icons/SystemUpdate'
import SubMenu from './SubMenu'
import { humanize, pluralize } from 'inflection'
import albumLists from '../album/albumLists'
import PlaylistsSubMenu from './PlaylistsSubMenu'
import LibrarySelector from '../common/LibrarySelector'
import config from '../config'

const useStyles = makeStyles((theme) => ({
  root: {
    marginTop: theme.spacing(1),
    marginBottom: theme.spacing(1),
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    paddingBottom: (props) => (props.addPadding ? '80px' : '20px'),
  },
  open: {
    width: 240,
  },
  closed: {
    width: 55,
  },
  active: {
    color: theme.palette.text.primary,
    fontWeight: 'bold',
  },
}))

const translatedResourceName = (resource, translate) =>
  translate(`resources.${resource.name}.name`, {
    smart_count: 2,
    _:
      resource.options && resource.options.label
        ? translate(resource.options.label, {
            smart_count: 2,
            _: resource.options.label,
          })
        : humanize(pluralize(resource.name)),
  })

const Menu = ({ dense = false }) => {
  const open = useSelector((state) => state.admin.ui.sidebarOpen)
  const translate = useTranslate()
  const queue = useSelector((state) => state.player?.queue)
  const classes = useStyles({ addPadding: queue.length > 0 })
  const resources = useSelector(getResources)

  const [state, setState] = useState({
    menuAlbumList: true,
    menuAudiobook: true,
    menuPlaylists: true,
    menuSharedPlaylists: true,
  })

  const handleToggle = (menu) => {
    setState((state) => ({ ...state, [menu]: !state[menu] }))
  }

  const renderResourceMenuItemLink = (resource) => (
    <MenuItemLink
      key={resource.name}
      to={`/${resource.name}`}
      activeClassName={classes.active}
      primaryText={translatedResourceName(resource, translate)}
      leftIcon={resource.icon || <ViewListIcon />}
      sidebarIsOpen={open}
      dense={dense}
    />
  )

  const renderAlbumMenuItemLink = (type, al) => {
    const resource = resources.find((r) => r.name === 'album')
    if (!resource) {
      return null
    }

    const albumListAddress = `/album/${type}`

    const name = translate(`resources.album.lists.${type || 'default'}`, {
      _: translatedResourceName(resource, translate),
    })

    return (
      <MenuItemLink
        key={albumListAddress}
        to={albumListAddress}
        activeClassName={classes.active}
        primaryText={name}
        leftIcon={al.icon || <ViewListIcon />}
        sidebarIsOpen={open}
        dense={dense}
        exact
      />
    )
  }

  const subItems = (subMenu) => (resource) =>
    resource.hasList && resource.options && resource.options.subMenu === subMenu

  return (
    <div
      className={clsx(classes.root, {
        [classes.open]: open,
        [classes.closed]: !open,
      })}
    >
      {open && <LibrarySelector />}
      <SubMenu
        handleToggle={() => handleToggle('menuAlbumList')}
        isOpen={state.menuAlbumList}
        sidebarIsOpen={open}
        name="menu.albumList"
        icon={<AlbumIcon />}
        dense={dense}
      >
        {Object.keys(albumLists).map((type) =>
          renderAlbumMenuItemLink(type, albumLists[type]),
        )}
      </SubMenu>
      {resources.filter(subItems(undefined)).map(renderResourceMenuItemLink)}
      <SubMenu
        handleToggle={() => handleToggle('menuAudiobook')}
        isOpen={state.menuAudiobook}
        sidebarIsOpen={open}
        name="menu.audiobook"
        icon={<MenuBookIcon />}
        dense={dense}
      >
        <MenuItemLink
          to="/audiobook"
          activeClassName={classes.active}
          primaryText="全部有声书"
          leftIcon={<span style={{ fontSize: 16 }}>📖</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=有声读物"
          activeClassName={classes.active}
          primaryText="有声读物"
          leftIcon={<span style={{ fontSize: 16 }}>📖</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=评书"
          activeClassName={classes.active}
          primaryText="评书"
          leftIcon={<span style={{ fontSize: 16 }}>🎤</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=相声"
          activeClassName={classes.active}
          primaryText="相声"
          leftIcon={<span style={{ fontSize: 16 }}>😂</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=戏曲"
          activeClassName={classes.active}
          primaryText="戏曲"
          leftIcon={<span style={{ fontSize: 16 }}>🎭</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=儿童"
          activeClassName={classes.active}
          primaryText="儿童"
          leftIcon={<span style={{ fontSize: 16 }}>👶</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook?genre=教育"
          activeClassName={classes.active}
          primaryText="教育"
          leftIcon={<span style={{ fontSize: 16 }}>🎓</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/audiobook/starred"
          activeClassName={classes.active}
          primaryText="收藏"
          leftIcon={<span style={{ fontSize: 16 }}>⭐</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
      </SubMenu>
      <MenuItemLink
        to="/narrator"
        activeClassName={classes.active}
        primaryText="演播者"
        leftIcon={<span style={{ fontSize: 18 }}>🎤</span>}
        sidebarIsOpen={open}
        dense={dense}
      />
      <MenuItemLink
          to="/settings/version"
          activeClassName={classes.active}
          primaryText="版本更新"
          leftIcon={<span style={{ fontSize: 18 }}>⚙️</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        <MenuItemLink
          to="/settings/app"
          activeClassName={classes.active}
          primaryText="APP管理"
          leftIcon={<span style={{ fontSize: 18 }}>📱</span>}
          sidebarIsOpen={open}
          dense={dense}
        />
        {config.devSidebarPlaylists && open ? (
        <>
          <Divider />
          <PlaylistsSubMenu
            state={state}
            setState={setState}
            sidebarIsOpen={open}
            dense={dense}
          />
        </>
      ) : (
        resources.filter(subItems('playlist')).map(renderResourceMenuItemLink)
      )}
    </div>
  )
}

export default Menu


