declare module 'jsencrypt' {
  interface JSEncryptOptions {
    default_key_size?: string
  }

  class JSEncrypt {
    constructor(options?: JSEncryptOptions)
    setPublicKey(publicKey: string): void
    setPrivateKey(privateKey: string): void
    getPublicKey(): string
    getPrivateKey(): string
    getKey(): void
    encrypt(input: string): string | false
    decrypt(input: string): string | false
  }

  export default JSEncrypt
}

