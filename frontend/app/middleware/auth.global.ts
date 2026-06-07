// Keeps the two routes in sync with the in-memory connection state. /login owns
// app boot + SSO, so this gate only redirects based on `authStore.connected`:
// disconnected users are pushed to /login; connected users never sit on it.
export default defineNuxtRouteMiddleware((to) => {
  if (import.meta.server)
    return

  const authStore = useAuthStore()

  if (authStore.connected) {
    if (to.path === '/login')
      return navigateTo('/')
  }
  else if (to.path !== '/login') {
    return navigateTo('/login')
  }
})
