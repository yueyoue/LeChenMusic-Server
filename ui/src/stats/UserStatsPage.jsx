import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardContent,
  Typography,
  Box,
  Tabs,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  Paper,
  Chip,
  LinearProgress,
} from '@material-ui/core';
import { Title, useNotify } from 'react-admin';

const API_BASE = '/api';

function getAuthHeaders() {
  const token = localStorage.getItem('token') || '';
  return { 'X-ND-Authorization': `Bearer ${token}` };
}

function formatDuration(seconds) {
  if (!seconds || seconds <= 0) return '0分钟';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) {
    return `${hours}小时${minutes}分钟`;
  }
  return `${minutes}分钟`;
}

function descendingComparator(a, b, orderBy) {
  const av = a[orderBy] ?? 0;
  const bv = b[orderBy] ?? 0;
  if (bv < av) return -1;
  if (bv > av) return 1;
  return 0;
}

function getComparator(order, orderBy) {
  return order === 'desc'
    ? (a, b) => descendingComparator(a, b, orderBy)
    : (a, b) => -descendingComparator(a, b, orderBy);
}

function stableSort(array, comparator) {
  const stabilized = array.map((el, index) => [el, index]);
  stabilized.sort((a, b) => {
    const order = comparator(a[0], b[0]);
    if (order !== 0) return order;
    return a[1] - b[1];
  });
  return stabilized.map((el) => el[0]);
}

const userColumns = [
  { id: 'userName', label: '用户名', numeric: false },
  { id: 'totalCount', label: '播放次数', numeric: true },
  { id: 'totalDuration', label: '总时长', numeric: true },
  { id: 'musicDuration', label: '音乐时长', numeric: true },
  { id: 'musicCount', label: '音乐次数', numeric: true },
  { id: 'audiobookDuration', label: '有声书时长', numeric: true },
  { id: 'audiobookCount', label: '有声书次数', numeric: true },
];

function UserStatsTab({ users }) {
  const [order, setOrder] = useState('desc');
  const [orderBy, setOrderBy] = useState('totalDuration');

  const handleSort = (property) => {
    const isAsc = orderBy === property && order === 'asc';
    setOrder(isAsc ? 'desc' : 'asc');
    setOrderBy(property);
  };

  const sorted = stableSort(users || [], getComparator(order, orderBy));

  // 汇总统计
  const totalUsers = sorted.length;
  const totalPlayCount = sorted.reduce((s, u) => s + (u.totalCount || 0), 0);
  const totalDuration = sorted.reduce((s, u) => s + (u.totalDuration || 0), 0);

  return (
    <Box>
      {/* 概览卡片 */}
      <Box display="flex" flexWrap="wrap" gap={2} mb={3}>
        <Box flex="1 1 200px" minWidth={200}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                总用户数
              </Typography>
              <Typography variant="h4">{totalUsers}</Typography>
            </CardContent>
          </Card>
        </Box>
        <Box flex="1 1 200px" minWidth={200}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                总播放次数
              </Typography>
              <Typography variant="h4">{totalPlayCount.toLocaleString()}</Typography>
            </CardContent>
          </Card>
        </Box>
        <Box flex="1 1 200px" minWidth={200}>
          <Card>
            <CardContent>
              <Typography color="textSecondary" gutterBottom variant="body2">
                总播放时长
              </Typography>
              <Typography variant="h4">{formatDuration(totalDuration)}</Typography>
            </CardContent>
          </Card>
        </Box>
      </Box>

      {/* 用户详情表格 */}
      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              {userColumns.map((col) => (
                <TableCell
                  key={col.id}
                  align={col.numeric ? 'right' : 'left'}
                  sortDirection={orderBy === col.id ? order : false}
                >
                  <TableSortLabel
                    active={orderBy === col.id}
                    direction={orderBy === col.id ? order : 'asc'}
                    onClick={() => handleSort(col.id)}
                  >
                    {col.label}
                  </TableSortLabel>
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {sorted.length === 0 ? (
              <TableRow>
                <TableCell colSpan={userColumns.length} align="center">
                  <Typography color="textSecondary">暂无播放数据，等待 APP 端上报...</Typography>
                </TableCell>
              </TableRow>
            ) : (
              sorted.map((row, idx) => (
                <TableRow key={row.userId || idx} hover>
                  <TableCell>{row.userName || row.userId || '-'}</TableCell>
                  <TableCell align="right">{(row.totalCount || 0).toLocaleString()}</TableCell>
                  <TableCell align="right">{formatDuration(row.totalDuration)}</TableCell>
                  <TableCell align="right">{formatDuration(row.musicDuration)}</TableCell>
                  <TableCell align="right">{row.musicCount || 0}</TableCell>
                  <TableCell align="right">{formatDuration(row.audiobookDuration)}</TableCell>
                  <TableCell align="right">{row.audiobookCount || 0}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}

function VersionStatsTab({ versions }) {
  const totalUsers = (versions || []).reduce((sum, v) => sum + (v.userCount || 0), 0);

  return (
    <Box>
      <Box mb={2}>
        <Typography variant="body2" color="textSecondary">
          统计各 APP 版本的用户分布，用于追踪版本更新情况
        </Typography>
      </Box>
      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>版本号</TableCell>
              <TableCell align="right">用户数</TableCell>
              <TableCell style={{ minWidth: 200 }}>占比</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(!versions || versions.length === 0) ? (
              <TableRow>
                <TableCell colSpan={3} align="center">
                  <Typography color="textSecondary">暂无版本数据，等待 APP 端上报...</Typography>
                </TableCell>
              </TableRow>
            ) : (
              versions.map((row, idx) => {
                const pct = totalUsers > 0 ? ((row.userCount / totalUsers) * 100) : 0;
                return (
                  <TableRow key={row.appVersion || idx} hover>
                    <TableCell>
                      <Chip label={row.appVersion || '-'} size="small" variant="outlined" color="primary" />
                    </TableCell>
                    <TableCell align="right">{(row.userCount || 0).toLocaleString()}</TableCell>
                    <TableCell>
                      <Box display="flex" alignItems="center" gap={1}>
                        <Box flex={1}>
                          <LinearProgress
                            variant="determinate"
                            value={pct}
                            style={{ height: 8, borderRadius: 4 }}
                          />
                        </Box>
                        <Typography variant="body2" color="textSecondary" style={{ minWidth: 48 }}>
                          {pct.toFixed(1)}%
                        </Typography>
                      </Box>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}

export default function UserStatsPage() {
  const [tab, setTab] = useState(0);
  const [loading, setLoading] = useState(false);
  const [users, setUsers] = useState(null);
  const [versions, setVersions] = useState(null);
  const notify = useNotify();

  const fetchUserStats = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/stats/users`, { headers: getAuthHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      // API 返回格式: { data: [...UserPlayStats] }
      setUsers(json.data || []);
    } catch (err) {
      notify(`获取用户统计失败: ${err.message}`, 'warning');
    } finally {
      setLoading(false);
    }
  }, [notify]);

  const fetchVersionStats = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${API_BASE}/stats/versions`, { headers: getAuthHeaders() });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const json = await res.json();
      // API 返回格式: { data: [...VersionStat] }
      setVersions(json.data || []);
    } catch (err) {
      notify(`获取版本统计失败: ${err.message}`, 'warning');
    } finally {
      setLoading(false);
    }
  }, [notify]);

  useEffect(() => {
    if (tab === 0 && users === null) fetchUserStats();
    if (tab === 1 && versions === null) fetchVersionStats();
  }, [tab, users, versions, fetchUserStats, fetchVersionStats]);

  return (
    <Box p={2}>
      <Title title="用户统计" />
      <Typography variant="h5" gutterBottom>
        用户统计
      </Typography>

      <Tabs
        value={tab}
        onChange={(_, v) => setTab(v)}
        indicatorColor="primary"
        textColor="primary"
        style={{ marginBottom: 16 }}
      >
        <Tab label="📊 用户播放统计" />
        <Tab label="📱 APP 版本统计" />
      </Tabs>

      {loading && <LinearProgress style={{ marginBottom: 8 }} />}

      {tab === 0 && <UserStatsTab users={users} />}
      {tab === 1 && <VersionStatsTab versions={versions} />}
    </Box>
  );
}
