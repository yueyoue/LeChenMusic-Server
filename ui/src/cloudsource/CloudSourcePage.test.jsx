import React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockHttpClient = vi.fn()
const mockStartScan = vi.fn()

vi.mock('../dataProvider/httpClient', () => ({
  default: (...args) => mockHttpClient(...args),
}))

vi.mock('../subsonic', () => ({
  default: { startScan: (...args) => mockStartScan(...args) },
}))

vi.mock('react-redux', () => ({
  useSelector: (fn) => fn({ activity: { scanStatus: { scanning: false } } }),
}))

import CloudSourcePage from './CloudSourcePage'

const gatewayList = {
  data: {
    gateways: [
      {
        name: 'nas',
        url: 'http://192.168.1.10:5244',
        host: '192.168.1.10:5244',
        enabled: true,
        check: {
          name: 'nas',
          ok: true,
          rootEntries: 3,
          checkedAt: '2026-09-30T12:00:00Z',
          source: 'startup',
        },
        libraries: [
          {
            id: 2,
            name: '网盘音乐',
            mediaType: 'music',
            path: 'openlist://192.168.1.10:5244/fnos/音乐',
            totalAlbums: 100,
            totalSongs: 90,
            lastScanAt: '2026-09-30T12:00:00Z',
          },
          {
            id: 3,
            name: '网盘有声书',
            mediaType: 'audiobook',
            path: 'openlist://192.168.1.10:5244/fnos/有声书',
            // 有声书：一部书 = 一个专辑，一个章节 = 一首歌曲，计数与文件库一致
            totalAlbums: 20,
            totalSongs: 300,
            lastScanAt: '2026-09-29T12:00:00Z',
          },
        ],
        totalAlbums: 120,
        totalSongs: 390,
        lastScanAt: '2026-09-30T12:00:00Z',
      },
      {
        name: 'quark',
        url: 'http://pan.example.com:5244',
        host: 'pan.example.com:5244',
        enabled: false,
        check: {
          name: 'quark',
          ok: false,
          error: 'connection refused',
          checkedAt: '2026-09-30T11:00:00Z',
          source: 'manual',
        },
        libraries: [],
        totalAlbums: 0,
        totalSongs: 0,
      },
      {
        name: 'pan',
        url: 'http://pan2.example.com:5244',
        host: 'pan2.example.com:5244',
        enabled: true,
        check: {
          name: 'pan',
          ok: false,
          error: 'timeout',
          checkedAt: '2026-09-30T11:30:00Z',
          source: 'manual',
        },
        libraries: [],
        totalAlbums: 5,
        totalSongs: 3,
      },
    ],
  },
}

describe('CloudSourcePage (P2-4 云源管理面板)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // httpClient（react-admin fetchJson）返回 { status, headers, body, json }，
    // 组件按 res.json?.data?.gateways 解析——mock 必须带 json 包装层
    mockHttpClient.mockResolvedValue({ json: gatewayList })
  })

  it('lists gateways with album counts, last scan and bound libraries', async () => {
    render(<CloudSourcePage />)

    await waitFor(() => expect(screen.getByText('nas')).toBeInTheDocument())
    expect(screen.getByText('quark')).toBeInTheDocument()
    // 「专辑数 / 歌曲数」在汇总卡片和云库表格表头各出现一次
    expect(screen.getAllByText('专辑数').length).toBeGreaterThan(0)
    expect(screen.getAllByText('歌曲数').length).toBeGreaterThan(0)
    expect(screen.getByText('120')).toBeInTheDocument() // 专辑数 totalAlbums
    expect(screen.getByText('390')).toBeInTheDocument() // 歌曲数 totalSongs（含 300 章有声书）
    expect(screen.getByText('网盘音乐')).toBeInTheDocument()
    expect(screen.getByText('网盘有声书')).toBeInTheDocument()
    // 有声书 media type chip
    expect(screen.getByText('有声书')).toBeInTheDocument()
  })

  it('shows warning alerts for failing and disabled gateways', async () => {
    render(<CloudSourcePage />)

    await waitFor(() => expect(screen.getByText('quark')).toBeInTheDocument())
    // pan：启用但连接异常 → 红条；quark：已停用 → 黄条（一个网关只渲染一条告警，停用文案优先）
    const alerts = screen.getAllByRole('alert')
    expect(alerts.some((a) => /连接异常/.test(a.textContent))).toBe(true)
    expect(alerts.some((a) => /已停用/.test(a.textContent))).toBe(true)
  })

  it('toggles the 启停 switch through the API', async () => {
    mockHttpClient.mockImplementation((url, options) => {
      if (options && options.method === 'PUT') {
        return Promise.resolve({ json: { data: { name: 'nas', enabled: false } } })
      }
      return Promise.resolve({ json: gatewayList })
    })
    render(<CloudSourcePage />)

    await waitFor(() => expect(screen.getByText('nas')).toBeInTheDocument())
    const switches = screen.getAllByRole('checkbox')
    // 第一个 switch 属于 nas（启用中）
    fireEvent.click(switches[0])

    await waitFor(() =>
      expect(mockHttpClient).toHaveBeenCalledWith(
        '/api/cloudsource/gateways/nas/enabled',
        expect.objectContaining({ method: 'PUT' }),
      ),
    )
  })

  it('triggers a single-source scan with the gateway libraries as targets', async () => {
    render(<CloudSourcePage />)

    await waitFor(() => expect(screen.getByText('nas')).toBeInTheDocument())
    const scanButtons = screen.getAllByRole('button', { name: /扫描云库/ })
    // quark 已停用 → 按钮禁用；nas 可用
    expect(scanButtons[0]).not.toBeDisabled()
    fireEvent.click(scanButtons[0])

    await waitFor(() =>
      expect(mockStartScan).toHaveBeenCalledWith({ target: ['2:', '3:'] }),
    )
  })

  it('disables scan for disabled gateways', async () => {
    render(<CloudSourcePage />)

    await waitFor(() => expect(screen.getByText('quark')).toBeInTheDocument())
    const scanButtons = screen.getAllByRole('button', { name: /扫描云库/ })
    // quark: 已停用且无绑定库 → 按钮禁用
    expect(scanButtons[1]).toBeDisabled()
  })
})
