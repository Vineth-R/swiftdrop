//@ts-check

const { join } = require('node:path');

/** @type {import('next').NextConfig} */
const nextConfig = {
  turbopack: {
    root: join(__dirname, '../..'),
  },
  ...(process.platform !== 'win32' || process.env.STANDALONE === 'true'
    ? {
        output: 'standalone',
        outputFileTracingRoot: join(__dirname, '../..'),
      }
    : {}),
};

module.exports = nextConfig;
