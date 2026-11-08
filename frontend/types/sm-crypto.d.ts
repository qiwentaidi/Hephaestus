declare module 'sm-crypto' {
  interface SM2KeyPair {
    publicKey: string
    privateKey: string
  }

  interface SM4Options {
    mode?: 'cbc' | 'ecb'
    iv?: string
    output?: 'string' | 'array'
  }

  const sm2: {
    generateKeyPairHex(): SM2KeyPair
    doEncrypt(msg: string, publicKey: string, cipherMode?: number): string
    doDecrypt(encryptData: string, privateKey: string, cipherMode?: number): string
    doSignature(msg: string, privateKey: string, options?: object): string
    doVerifySignature(msg: string, signHex: string, publicKey: string, options?: object): boolean
  }

  const sm3: (msg: string) => string

  const sm4: {
    encrypt(msg: string, key: string, options?: SM4Options): string | number[]
    decrypt(encryptData: string, key: string, options?: SM4Options): string | number[]
  }

  export { sm2, sm3, sm4 }
  const smCrypto: { sm2: typeof sm2; sm3: typeof sm3; sm4: typeof sm4 }
  export default smCrypto
}
