import React, { useEffect, useRef, useState } from 'react'

// 有声书封面图片，带「加载失败自动重试」。
//
// 为什么需要重试：服务端解析封面（尤其是网盘书目）可能要几秒，但请求不会等它——
// 拿不到就先回 404，后台解析完落缓存，下一次请求才给图（见 server/nativeapi/cover_resolver.go）。
// 所以这里的 onError 多半不是「这本书没封面」，而是「服务端还没解析好」。
//
// 重试靠重新挂载 <img>（key 变化）而不是改 src：React 对相同的 src 不会重新发起请求，
// 而一个全新的 <img> 元素会重新走一遍 HTTP 缓存协商，从而拿到后台刚落盘的封面。
// 退避时间翻倍，几次之后就认输显示占位图，不会无限打服务端。
const RETRY_DELAYS = [2000, 4000, 8000, 16000]

export const CoverImage = ({
  src,
  alt,
  component: Component = 'img',
  fallback = null,
  onLoad,
  onError,
  ...rest
}) => {
  const [attempt, setAttempt] = useState(0)
  const [failed, setFailed] = useState(false)
  const timer = useRef(null)

  useEffect(() => () => clearTimeout(timer.current), [])

  if (failed) return fallback

  return (
    <Component
      key={attempt}
      src={src}
      alt={alt}
      loading="lazy"
      decoding="async"
      onLoad={onLoad}
      onError={(e) => {
        if (onError) onError(e)
        if (attempt >= RETRY_DELAYS.length) {
          setFailed(true)
          return
        }
        clearTimeout(timer.current)
        timer.current = setTimeout(
          () => setAttempt((a) => a + 1),
          RETRY_DELAYS[attempt],
        )
      }}
      {...rest}
    />
  )
}
