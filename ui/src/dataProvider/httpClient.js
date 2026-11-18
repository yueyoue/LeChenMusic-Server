import { fetchUtils } from 'react-admin'
import { v4 as uuidv4 } from 'uuid'
import { baseUrl } from '../utils'
import config from '../config'
import { jwtDecode } from 'jwt-decode'
import { removeHomeCache } from '../utils/removeHomeCache'

const customAuthorizationHeader = 'X-ND-Authorization'
export const clientUniqueIdHeader = 'X-ND-Client-Unique-Id'
export const clientUniqueId = uuidv4()
// 管理端标识：服务端据此把“上游要口令/被风控”这类需要管理员判断的提示返回给网页，
// 而 APP 不发送该头，普通用户只会看到中性的失败文案。
export const onlineClientHeader = 'X-Online-Client'
export const onlineClientWebAdmin = 'web-admin'

const httpClient = (url, options = {}) => {
  url = baseUrl(url)
  if (!options.headers) {
    options.headers = new Headers({ Accept: 'application/json' })
  }
  options.headers.set(clientUniqueIdHeader, clientUniqueId)
  options.headers.set(onlineClientHeader, onlineClientWebAdmin)
  const token = localStorage.getItem('token')
  if (token) {
    options.headers.set(customAuthorizationHeader, `Bearer ${token}`)
  }
  return fetchUtils.fetchJson(url, options).then((response) => {
    const token = response.headers.get(customAuthorizationHeader)
    if (token) {
      const decoded = jwtDecode(token)
      localStorage.setItem('token', token)
      localStorage.setItem('userId', decoded.uid)
      // Avoid going to create admin dialog after logout/login without a refresh
      config.firstTime = false
      removeHomeCache()
    }
    return response
  })
}

export default httpClient
