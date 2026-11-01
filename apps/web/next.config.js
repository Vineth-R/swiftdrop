//@ts-check

const { join } = require('node:path');

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  outputFileTracingRoot: join(__dirname, '../..'),
};

module.exports = nextConfig;
