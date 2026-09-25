// How a library's `path` column encodes a cloud media source (网盘文件夹), and how to
// split/combine it in the admin wizard.
//
// ⚠️ This must stay in sync with the backend's single source of truth:
//    core/cloudsource/storage.go -> BuildURI()
// The wizard writes `path` straight to the DB, so whatever we compose here has to be
// byte-identical to what BuildURI() would have produced.

export const CLOUD_SCHEME = 'openlist://'

const SOURCE_LOCAL = 'local'
const SOURCE_CLOUD = 'cloud'

/** A library path refers to a cloud source when it uses the openlist:// scheme. */
export const isCloudPath = (path) =>
  String(path || '')
    .trim()
    .toLowerCase()
    .startsWith(CLOUD_SCHEME)

/** Keep only `host:port`, dropping any scheme, trailing slash, query or fragment. */
export const canonicalHost = (address) => {
  let host = String(address || '')
    .trim()
    .replace(/\/+$/, '')
  host = host.replace(/^[a-z][a-z0-9+.-]*:\/\//i, '') // drop http://, https://, ...
  host = host.split(/[/?#]/)[0].toLowerCase() // keep host:port only
  return host
}

/**
 * Compose the library `path` from the two wizard inputs.
 *
 *   buildCloudPath('http://192.168.1.10:5244', 'fnos/音乐')
 *     -> 'openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90'
 *
 * Each path segment is escaped separately (never the path as a whole) and escaped exactly
 * once — BuildURI() relies on url.URL.String() doing the per-component escaping, so
 * pre-escaping the whole path would double-encode non-ASCII segments.
 */
export const buildCloudPath = (address, remotePath) => {
  const host = canonicalHost(address)
  if (!host) {
    throw new Error('OpenList address is required')
  }
  const path = String(remotePath || '').replace(/^\/+|\/+$/g, '')
  if (!path) {
    return `${CLOUD_SCHEME}${host}`
  }
  return `${CLOUD_SCHEME}${host}/${path
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/')}`
}

const safeDecode = (value) => {
  try {
    return decodeURIComponent(value)
  } catch {
    // A hand-written library path may contain stray '%' characters; use it as-is.
    return value
  }
}

/**
 * Split a stored library `path` back into the two wizard inputs.
 *
 *   parseCloudPath('openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90')
 *     -> { address: '192.168.1.10:5244', remotePath: 'fnos/音乐' }
 */
export const parseCloudPath = (path) => {
  const raw = String(path || '').trim()
  if (!isCloudPath(raw)) {
    return { address: '', remotePath: '' }
  }
  const rest = raw.slice(CLOUD_SCHEME.length)
  const slash = rest.indexOf('/')
  const address = slash === -1 ? rest : rest.slice(0, slash)
  const remotePath =
    slash === -1 ? '' : rest.slice(slash + 1).split('/').map(safeDecode).join('/')
  return { address, remotePath }
}

/** Which kind of source a library path refers to. */
export const sourceTypeOf = (path) =>
  isCloudPath(path) ? SOURCE_CLOUD : SOURCE_LOCAL

/**
 * Turn wizard form values into the library resource payload: composes `path` for a cloud
 * library and drops the wizard-only fields so they never reach the API.
 */
export const toLibraryPayload = (values) => {
  const { sourceType, openlistAddress, remotePath, ...rest } = values || {}
  if (sourceType === SOURCE_CLOUD) {
    return { ...rest, path: buildCloudPath(openlistAddress, remotePath) }
  }
  // Local folder: `path` is used exactly as typed — nothing else changes.
  return rest
}

export const SOURCE_TYPES = { local: SOURCE_LOCAL, cloud: SOURCE_CLOUD }
