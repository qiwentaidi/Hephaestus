import os from 'node:os'

/** @type {import('next').NextConfig} */

const isStaticExport = process.env.STATIC_EXPORT === 'true'
const localDevOrigins = Object.values(os.networkInterfaces())
  .flat()
  .filter((address) => address?.family === 'IPv4' && !address.internal)
  .map((address) => address.address)

const nextConfig = {
  allowedDevOrigins: localDevOrigins,

  typescript: {
    ignoreBuildErrors: true,
  },
  images: {
    unoptimized: true,
  },

  ...(isStaticExport && {
    output: 'export',
    trailingSlash: false,   // ❗❗关键修复
    distDir: 'out',
  }),

  turbopack: {},

  webpack: (config) => {
    config.module = {
      ...config.module,
      unknownContextCritical: false,
      unknownContextRegExp: /^\.\/.*$/,
      unknownContextRequest: '.',
    }
    return config
  },
}

export default nextConfig
