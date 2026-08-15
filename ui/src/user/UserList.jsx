import React, { useState, useEffect } from 'react'
import {
  BooleanField,
  Datagrid,
  Filter,
  SearchInput,
  SimpleList,
  TextField,
} from 'react-admin'
import { useMediaQuery, Typography } from '@material-ui/core'
import { List, DateField } from '../common'
import httpClient from '../dataProvider/httpClient'
import { REST_URL } from '../consts'

const UserFilter = (props) => (
  <Filter {...props} variant={'outlined'}>
    <SearchInput id="search" source="name" alwaysOn />
  </Filter>
)

const formatDuration = (seconds) => {
  if (!seconds || seconds <= 0) return '-'
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}小时${minutes}分钟`
  if (minutes > 0) return `${minutes}分钟`
  return `${seconds}秒`
}

const UserList = (props) => {
  const isXsmall = useMediaQuery((theme) => theme.breakpoints.down('xs'))
  const [userStats, setUserStats] = useState({})

  useEffect(() => {
    httpClient(`${REST_URL}/stats/users`)
      .then((res) => {
        const statsMap = {}
        ;(res.json?.data || []).forEach((s) => {
          statsMap[s.userId] = s
        })
        setUserStats(statsMap)
      })
      .catch(() => {})
  }, [])

  const ListeningDurationField = ({ record }) => {
    const stats = userStats[record.id]
    return (
      <Typography variant="body2" style={{ fontSize: 13 }}>
        {stats ? formatDuration(stats.totalDuration) : '-'}
      </Typography>
    )
  }

  return (
    <List
      {...props}
      sort={{ field: 'userName', order: 'ASC' }}
      exporter={false}
      bulkActionButtons={false}
      filters={<UserFilter />}
    >
      {isXsmall ? (
        <SimpleList
          primaryText={(record) => record.userName}
          secondaryText={(record) => {
            const stats = userStats[record.id]
            const duration = stats ? formatDuration(stats.totalDuration) : ''
            const lastLogin = record.lastLoginAt ? new Date(record.lastLoginAt).toLocaleString() : ''
            return duration && lastLogin ? `${duration} · ${lastLogin}` : duration || lastLogin
          }}
          tertiaryText={(record) => (record.isAdmin ? '[admin]️' : '')}
        />
      ) : (
        <Datagrid rowClick="edit">
          <TextField source="userName" />
          <TextField source="name" />
          <BooleanField source="isAdmin" />
          <ListeningDurationField label="听歌时长" />
          <DateField source="lastLoginAt" sortByOrder={'DESC'} />
          <DateField source="lastAccessAt" sortByOrder={'DESC'} />
          <DateField source="updatedAt" sortByOrder={'DESC'} />
        </Datagrid>
      )}
    </List>
  )
}

export default UserList
