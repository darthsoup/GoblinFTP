import { ApiError } from '~/types/api'

let epoch = 0
const requests = new Set<AbortController>()
const objectUrls = new Set<string>()

export function connectionEpoch() {
  return epoch
}
export function assertConnection(captured: number) {
  if (captured !== epoch)
    throw new ApiError('ERR_STALE_CONNECTION', 'This action belongs to a previous connection')
}
export function connectionRequest() {
  const controller = new AbortController()
  requests.add(controller)
  return { controller, release: () => requests.delete(controller) }
}
export function rememberObjectUrl(url: string) {
  objectUrls.add(url)
  return url
}
export function invalidateConnection() {
  epoch++
  for (const controller of requests) controller.abort()
  requests.clear()
  for (const url of objectUrls) URL.revokeObjectURL(url)
  objectUrls.clear()
}

export function forgetObjectUrl(url: string) {
  objectUrls.delete(url)
  URL.revokeObjectURL(url)
}
