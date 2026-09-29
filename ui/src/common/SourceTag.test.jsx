import React from 'react'
import { render, screen } from '@testing-library/react'
import { SourceTag } from './SourceTag'

// Mock react-admin 的 useTranslate，返回 key 本身便于断言
vi.mock('react-admin', () => ({
  useTranslate: () => (key) => key,
}))

describe('SourceTag', () => {
  it('renders the cloud tag for an openlist:// library path', () => {
    render(<SourceTag libraryPath="openlist://192.168.1.10:5244/fnos/音乐" />)
    expect(screen.getByText('resources.library.cloud.tagCloud')).toBeInTheDocument()
  })

  it('renders the local tag for a local library path', () => {
    render(<SourceTag libraryPath="/vol1/music" />)
    expect(screen.getByText('resources.library.cloud.tagLocal')).toBeInTheDocument()
  })

  it('renders the local tag when the path is missing (defensive default)', () => {
    render(<SourceTag libraryPath={undefined} />)
    expect(screen.getByText('resources.library.cloud.tagLocal')).toBeInTheDocument()
  })

  it('merges custom style over the default colours', () => {
    render(<SourceTag libraryPath="/vol1/music" style={{ marginTop: 4 }} />)
    const chip = screen.getByText('resources.library.cloud.tagLocal').closest('div')
    expect(chip).toHaveStyle({ marginTop: '4px' })
  })
})
