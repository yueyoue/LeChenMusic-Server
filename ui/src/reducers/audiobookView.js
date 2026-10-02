import { AUDIOBOOK_MODE_GRID, AUDIOBOOK_MODE_TABLE } from '../actions'

// 与 albumView 一致：grid=true 走封面网格，grid=false 走表格。
export const audiobookViewReducer = (
  previousState = {
    grid: true,
  },
  payload,
) => {
  const { type } = payload
  switch (type) {
    case AUDIOBOOK_MODE_GRID:
    case AUDIOBOOK_MODE_TABLE:
      return { ...previousState, grid: type === AUDIOBOOK_MODE_GRID }
    default:
      return previousState
  }
}
