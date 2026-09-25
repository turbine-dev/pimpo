// Zodim runs locally, so the app shell is always fresh from the server.
// This worker exists only so the app can be installed; it caches nothing.
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))
self.addEventListener('fetch', () => {})
