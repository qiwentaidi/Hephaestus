// 密码破解/解密函数

// 帆软v8解密掩码
const PASSWORD_MASK_ARRAY: number[] = [19, 78, 10, 15, 100, 213, 43, 23]

// FineReport v8 密码解密
export const decryptFinereport = (input: string): string => {
  if (input.length <= 3) {
    return '[解密失败: 密码格式错误，示例: ___0072002a00670066000a]'
  }
  let result = ''
  const temp = input.substring(3)
  for (let i = 0; i < temp.length / 4; i++) {
    const c1 = parseInt(temp.substring(i * 4, (i + 1) * 4), 16)
    const c2 = c1 ^ PASSWORD_MASK_ARRAY[i % 8]
    result += String.fromCharCode(c2)
  }
  return result || '[解密失败]'
}

// Seeyon OA 密码解密
export const decryptSeeyon = (input: string): string => {
  const pass = input.replace(/\//g, '')
  const p = pass.split('.0')
  if (p.length <= 1) {
    return '[解密失败: 密码格式错误，示例: /1.0/UWJ0dHgxc2U=]'
  }
  let result = ''
  const iv = parseInt(p[0])
  try {
    const password = atob(p[1])
    for (let i = 0; i < password.length; i++) {
      const char = password.charCodeAt(i)
      result += String.fromCharCode(char - iv)
    }
    return result || '[解密失败]'
  } catch {
    return '[解密失败: Base64解码错误]'
  }
}

// AES/DES JSON Key 转换
export const decryptAesDesJson = (input: string): string => {
  try {
    const dataDict = JSON.parse(input)
    if (dataDict.words && Array.isArray(dataDict.words) && dataDict.sigBytes) {
      const { words, sigBytes } = dataDict
      if (sigBytes < 0) return '[有效字节数不能为负数]'
      const buffer = new ArrayBuffer(words.length * 4)
      const dataView = new DataView(buffer)
      words.forEach((num: number, index: number) => {
        dataView.setUint32(index * 4, num, false)
      })
      const byteSequence = new Uint8Array(buffer, 0, sigBytes)
      return new TextDecoder('utf-8').decode(byteSequence)
    }
    return '[JSON格式错误，示例: {"words": [1161312566,808858179], "sigBytes": 8}]'
  } catch {
    return '[JSON解析错误]'
  }
}

// WinSCP 密码解密
export const decryptWinSCP = (input: string): string => {
  const WSCP_SIMPLE_MAGIC = 0xa3
  const WSCP_SIMPLE_STRING = '0123456789ABCDEF'
  const WSCP_SIMPLE_FLAG = 0xff
  const WSCP_SIMPLE_INTERNAL = 0x00

  function decrypt(username: string, hostname: string, encodedPassword: string): string {
    if (!encodedPassword.match(/[A-F0-9]+/)) return ''
    let WSCP_CHARS = encodedPassword.split('')
    
    const _next = () => {
      if (WSCP_CHARS.length === 0) return WSCP_SIMPLE_INTERNAL
      const a = WSCP_SIMPLE_STRING.indexOf(WSCP_CHARS.shift()!)
      const b = WSCP_SIMPLE_STRING.indexOf(WSCP_CHARS.shift()!)
      return WSCP_SIMPLE_FLAG & ~(((a << 4) + b << 0) ^ WSCP_SIMPLE_MAGIC)
    }

    const result: string[] = []
    const key = username + hostname
    const flag = _next()
    let length: number

    if (flag === WSCP_SIMPLE_FLAG) {
      _next()
      length = _next()
    } else {
      length = flag
    }

    WSCP_CHARS = WSCP_CHARS.slice(_next() * 2)
    for (let i = 0; i < length; i++) {
      result.push(String.fromCharCode(_next()))
    }

    if (flag === WSCP_SIMPLE_FLAG) {
      const valid = result.slice(0, key.length).join('')
      if (valid !== key) return ''
      return result.slice(key.length).join('')
    }
    return result.join('')
  }

  const sessions = input.split('\n[').filter(s => s.includes('Sessions\\'))
  if (sessions.length === 0) return '[未找到WinSCP会话信息]'
  
  const results = sessions.map(session => {
    const sessionName = decodeURIComponent(session.split(']')[0].replace('Sessions\\', '').replace('[', ''))
    const lines = session.split('\n')
    const usernameLine = lines.find(l => l.startsWith('UserName='))
    const hostnameLine = lines.find(l => l.startsWith('HostName='))
    const passwordLine = lines.find(l => l.startsWith('Password='))

    if (usernameLine && hostnameLine && passwordLine) {
      const username = usernameLine.split('=')[1]?.trim() || ''
      const hostname = hostnameLine.split('=')[1]?.trim() || ''
      const encodedPassword = passwordLine.split('=')[1]?.trim() || ''
      const decodedPassword = decrypt(username, hostname, encodedPassword)
      return `[${sessionName}]\n用户: ${username}\n主机: ${hostname}\n密码: ${decodedPassword || '解密失败'}`
    }
    return `[${sessionName}] 信息不完整`
  })
  return results.join('\n\n')
}

