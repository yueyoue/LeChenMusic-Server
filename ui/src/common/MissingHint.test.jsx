import React from 'react'
import { render, screen } from '@testing-library/react'
import { MissingHint } from './MissingHint'

// Mock react-admin 的 useTranslate，返回 key 本身便于断言
vi.mock('react-admin', () => ({
  useTranslate: () => (key) => key,
}))

const Row = React.forwardRef((props, ref) => (
  <tr ref={ref} {...props}>
    <td>row</td>
  </tr>
))

describe('MissingHint', () => {
  it('shows the missing hint on hover for a missing record', () => {
    render(
      <MissingHint record={{ missing: true }}>
        <Row />
      </MissingHint>,
    )
    expect(screen.getByText('row')).toBeInTheDocument()
  })

  it('passes children through untouched for present records', () => {
    const { container } = render(
      <MissingHint record={{ missing: false }}>
        <Row />
      </MissingHint>,
    )
    // 非 missing：不做任何 Tooltip 包裹（保持行内 DOM 结构零变化）
    expect(container.querySelector('tr')).toBeInTheDocument()
  })

  it('passes children through untouched when record is undefined (defensive default)', () => {
    const { container } = render(
      <MissingHint record={undefined}>
        <Row />
      </MissingHint>,
    )
    expect(container.querySelector('tr')).toBeInTheDocument()
  })
})
