//@ts-check

const { join } = require('node:path');

// Single source of truth, shared with src/lib/constants.ts.
const { base: apiBase } = require('./src/lib/api-config.json');

/**
 * `nx dev` serves the app on :3000 while the API lives behind the Nginx
 * gateway on :8080. Proxying /api/v1 through Next keeps the browser on one
 * origin, so the HttpOnly session cookies are first-party and the code never
 * needs a different URL in development. Behind the gateway (production and
 * Docker) Nginx already routes /api/v1 to the services, so nothing is added.
 * Set API_PROXY_TARGET to point development somewhere else.
 */
const apiProxyTarget =
  process.env.API_PROXY_TARGET ||
  (process.env.NODE_ENV === 'production' ? '' : 'http://127.0.0.1:8080');

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  outputFileTracingRoot: join(__dirname, '../..'),
  // `nx dev` and Playwright reach the dev server over 127.0.0.1. Next 16.4
  // blocks cross-origin dev resources by default, so allow that loopback host.
  allowedDevOrigins: ['127.0.0.1'],
  // Do not generate an AI-agent instruction file during local development.
  agentRules: false,
  async rewrites() {
    if (!apiProxyTarget) return [];
    return [
      {
        source: `${apiBase}/:path*`,
        destination: `${apiProxyTarget}${apiBase}/:path*`,
      },
    ];
  },
};

module.exports = nextConfig;
