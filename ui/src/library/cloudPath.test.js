import {
  CLOUD_SCHEME,
  buildCloudPath,
  canonicalHost,
  isCloudPath,
  parseCloudPath,
  sourceTypeOf,
  toLibraryPayload,
} from './cloudPath.js'

describe('buildCloudPath', () => {
  // These two examples are the contract with the backend: they must match
  // core/cloudsource/storage.go -> BuildURI() exactly.
  it('builds the documented example with a Chinese path', () => {
    expect(buildCloudPath('http://192.168.1.10:5244', 'fnos/音乐')).toEqual(
      'openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90',
    )
  })

  it('escapes every segment exactly once (never the path as a whole)', () => {
    // Escaping the whole path would turn '%' into '%25' and double-encode.
    const path = buildCloudPath('http://192.168.1.10:5244', '有声书/鬼吹灯')
    expect(path).toEqual('openlist://192.168.1.10:5244/%E6%9C%89%E5%A3%B0%E4%B9%A6/%E9%AC%BC%E5%90%B9%E7%81%AF')
    expect(path).not.toContain('%25')
  })

  it('drops the scheme, trailing slashes and lowercases the host', () => {
    expect(buildCloudPath('https://NAS.Local:5244/', 'fnos/音乐')).toEqual(
      'openlist://nas.local:5244/fnos/%E9%9F%B3%E4%B9%90',
    )
    expect(buildCloudPath('  nas.local:5244  ', 'fnos')).toEqual('openlist://nas.local:5244/fnos')
  })

  it('ignores query/fragment junk in the address', () => {
    expect(canonicalHost('http://192.168.1.10:5244/#/settings')).toEqual('192.168.1.10:5244')
  })

  it('trims slashes around the remote path', () => {
    expect(buildCloudPath('http://h:1', '/fnos/音乐/')).toEqual('openlist://h:1/fnos/%E9%9F%B3%E4%B9%90')
    expect(buildCloudPath('http://h:1', '  /fnos/  ')).toEqual('openlist://h:1/fnos')
  })

  it('allows a library rooted at the gateway', () => {
    expect(buildCloudPath('http://h:1', '')).toEqual('openlist://h:1')
  })

  it('rejects an empty address', () => {
    expect(() => buildCloudPath('', 'fnos')).toThrow(/address/i)
    expect(() => buildCloudPath('   ', 'fnos')).toThrow(/address/i)
    expect(() => buildCloudPath('http://', 'fnos')).toThrow(/address/i)
  })
})

describe('parseCloudPath', () => {
  it('splits the documented example back into the two wizard inputs', () => {
    expect(parseCloudPath('openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90')).toEqual({
      address: '192.168.1.10:5244',
      remotePath: 'fnos/音乐',
    })
  })

  it('round-trips through buildCloudPath', () => {
    const cases = [
      ['http://192.168.1.10:5244', 'fnos/音乐'],
      ['http://nas.local:5244', '有声书/鬼吹灯/第一部'],
      ['https://h:8443', 'a/b c/d%e'],
      ['h:1', ''],
    ]
    for (const [address, remotePath] of cases) {
      const built = buildCloudPath(address, remotePath)
      const parsed = parseCloudPath(built)
      expect(parsed.address).toEqual(canonicalHost(address))
      expect(parsed.remotePath).toEqual(remotePath)
      // Re-saving an untouched library must not rewrite its path.
      expect(buildCloudPath(parsed.address, parsed.remotePath)).toEqual(built)
    }
  })

  it('tolerates a hand-written path with unescaped characters', () => {
    expect(parseCloudPath('openlist://h:1/fnos/音乐')).toEqual({
      address: 'h:1',
      remotePath: 'fnos/音乐',
    })
  })

  it('does not throw on a malformed escape sequence', () => {
    expect(() => parseCloudPath('openlist://h:1/100%/x')).not.toThrow()
    expect(parseCloudPath('openlist://h:1/100%/x').remotePath).toEqual('100%/x')
  })

  it('returns empty fields for a non-cloud path', () => {
    expect(parseCloudPath('/mnt/music')).toEqual({ address: '', remotePath: '' })
    expect(parseCloudPath('')).toEqual({ address: '', remotePath: '' })
    expect(parseCloudPath(undefined)).toEqual({ address: '', remotePath: '' })
  })
})

describe('isCloudPath / sourceTypeOf', () => {
  it('detects the cloud scheme case-insensitively', () => {
    expect(isCloudPath('openlist://h:1/a')).toBe(true)
    expect(isCloudPath('OPENLIST://h:1/a')).toBe(true)
    expect(isCloudPath('  openlist://h:1/a  ')).toBe(true)
    expect(isCloudPath('/mnt/music')).toBe(false)
    expect(isCloudPath('file:///mnt/music')).toBe(false)
    expect(isCloudPath('')).toBe(false)
    expect(sourceTypeOf('openlist://h:1/a')).toEqual('cloud')
    expect(sourceTypeOf('/mnt/music')).toEqual('local')
  })
})

describe('toLibraryPayload', () => {
  it('composes `path` for a cloud library and drops the wizard-only fields', () => {
    expect(
      toLibraryPayload({
        name: '网盘音乐',
        sourceType: 'cloud',
        cloudAddress: 'http://192.168.1.10:5244',
        cloudRemotePath: 'fnos/音乐',
        defaultNewUsers: true,
      }),
    ).toEqual({
      name: '网盘音乐',
      path: 'openlist://192.168.1.10:5244/fnos/%E9%9F%B3%E4%B9%90',
      defaultNewUsers: true,
    })
  })

  it('leaves a local library completely untouched', () => {
    // Regression guard: local is the default and must keep behaving exactly as before.
    const values = {
      name: '本地音乐',
      path: '/mnt/music',
      sourceType: 'local',
      cloudAddress: 'ignored',
      cloudRemotePath: 'ignored',
      defaultNewUsers: false,
    }
    expect(toLibraryPayload(values)).toEqual({
      name: '本地音乐',
      path: '/mnt/music',
      defaultNewUsers: false,
    })
  })

  it('keeps the model-owned `remotePath` field in the payload', () => {
    // Regression guard for the edit-form echo bug: model.Library has a legacy
    // `remotePath` column that must NOT be confused with (or stripped alongside)
    // the wizard's `cloudRemotePath` field.
    expect(
      toLibraryPayload({
        name: 'x',
        path: 'openlist://h:1/a',
        remotePath: '',
        sourceType: 'cloud',
        cloudAddress: 'h:1',
        cloudRemotePath: 'a',
      }),
    ).toEqual({
      name: 'x',
      path: 'openlist://h:1/a',
      remotePath: '',
    })
  })

  it('keeps `path` as typed when no source type was chosen', () => {
    expect(toLibraryPayload({ name: 'x', path: '/mnt/music' })).toEqual({
      name: 'x',
      path: '/mnt/music',
    })
  })

  it('surfaces a missing address as a validation error', () => {
    expect(() =>
      toLibraryPayload({ sourceType: 'cloud', openlistAddress: '', remotePath: 'fnos' }),
    ).toThrow(/address/i)
  })
})

describe('CLOUD_SCHEME', () => {
  it('matches what the backend registers', () => {
    expect(CLOUD_SCHEME).toEqual('openlist://')
  })
})
