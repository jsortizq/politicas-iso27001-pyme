/** @type {import('next').NextConfig} */

// The browser only ever talks to this Next.js origin; /api and /auth are
// proxied to the Go backend. Keeping everything same-origin lets the
// HttpOnly session cookie work without any cross-site configuration.
const backend = process.env.API_URL || 'http://localhost:8080';

const nextConfig = {
  async rewrites() {
    return [
      { source: '/api/:path*', destination: `${backend}/api/:path*` },
      { source: '/auth/:path*', destination: `${backend}/auth/:path*` },
    ];
  },
};

module.exports = nextConfig;
