const { createProxyMiddleware } = require('http-proxy-middleware')

/**
 * Proxy configuration for development
 * This allows us to bypass CORS by proxying requests through the dev server
 *
 * Usage:
 *   PROXY_TARGET="https://sippy-auth.dptools.openshift.org" \
 *   REACT_APP_OAUTH_TOKEN="your-token" \
 *   npm start
 *
 * Then access the app at http://localhost:3000/sippy-ng
 * All /api requests will be proxied to PROXY_TARGET
 */
module.exports = function (app) {
  console.log('=== setupProxy.js loaded ===')
  console.log('PROXY_TARGET:', process.env.PROXY_TARGET)
  console.log('REACT_APP_OAUTH_TOKEN:', process.env.REACT_APP_OAUTH_TOKEN ? 'SET' : 'NOT SET')
  
  const target = process.env.PROXY_TARGET

  // Only enable proxy if PROXY_TARGET is set
  if (!target) {
    console.log('❌ No PROXY_TARGET set, proxy disabled')
    console.log(
      'To use proxy: PROXY_TARGET="https://sippy-auth.dptools.openshift.org" npm start'
    )
    return
  }

  const token = process.env.REACT_APP_OAUTH_TOKEN

  console.log(`\n🔄 Proxy enabled:`)
  console.log(`   Target: ${target}`)
  console.log(`   Auth: ${token ? '✓ OAuth token configured' : '✗ No OAuth token'}`)
  console.log(`   Proxying: /api/* → ${target}/api/*\n`)

  const proxyOptions = {
    target: target,
    changeOrigin: true,
    ws: true, // Enable WebSocket proxying
    secure: true, // Verify SSL certificates
    logLevel: 'debug', // Changed to debug for more info

    // Add OAuth token to proxied requests if configured
    onProxyReq: (proxyReq, req, res) => {
      // Use stderr to ensure logs are visible
      process.stderr.write(`\n>>> [Proxy] Intercepted: ${req.method} ${req.url}\n`)
      process.stderr.write(`>>> [Proxy] Token available: ${!!token}\n`)
      
      if (token) {
        proxyReq.setHeader('Authorization', `Bearer ${token}`)
        process.stderr.write(`>>> [Proxy] Set Authorization header\n`)
        process.stderr.write(`>>> [Proxy] Token preview: ${token.substring(0, 20)}...\n`)
      } else {
        process.stderr.write(`>>> [Proxy] ERROR: No token!\n`)
      }
      process.stderr.write(`>>> [Proxy] Target: ${target}${req.url}\n\n`)
    },

    // Handle WebSocket upgrade with authentication
    onProxyReqWs: (proxyReq, req, socket, options, head) => {
      console.log(`\n>>> [WebSocket] Intercepted: ${req.url}`)
      if (token) {
        proxyReq.setHeader('Authorization', `Bearer ${token}`)
        console.log(`>>> [WebSocket] Added Authorization header\n`)
      }
    },

    // Log errors
    onError: (err, req, res) => {
      console.error('\n❌ [Proxy] Error:', err.message)
      console.error('Request URL:', req.url, '\n')
    },
  }

  console.log('Installing proxy middleware for /api routes...')
  
  try {
    // Create middleware with explicit context and options
    const middleware = createProxyMiddleware('/api', proxyOptions)
    app.use(middleware)
    console.log('✅ Proxy middleware configured successfully')
    console.log('   Middleware registered for: /api')
    console.log('   Test it: curl http://localhost:3000/api/report_date\n')
  } catch (error) {
    console.error('❌ Failed to configure proxy middleware:')
    console.error(error)
    throw error
  }
}

