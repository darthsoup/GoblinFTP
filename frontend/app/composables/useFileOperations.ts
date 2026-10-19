const SIZE_MUTATIONS = new Set([
  '/api/files',
  '/api/files/rename',
  '/api/files/copy',
  '/api/files/directory',
  '/api/files/permissions',
  '/api/files/upload',
  '/api/files/upload/commit',
  '/api/files/write',
  '/api/files/extract',
  '/api/files/compress',
])

export function useFileOperations() {
  const api = useApi()

  async function mutate<T>(path: string, request: () => Promise<T>): Promise<T> {
    if (!SIZE_MUTATIONS.has(path.split('?')[0]!))
      return request()
    const sizes = useDirectorySizesStore()
    const connection = sizes.connectionGeneration
    sizes.invalidate(connection)
    try {
      return await request()
    }
    finally {
      sizes.invalidate(connection)
    }
  }

  return {
    ...api,
    post: <T>(path: string, body?: unknown, options?: Parameters<typeof api.post>[2]) => mutate(path, () => options ? api.post<T>(path, body, options) : api.post<T>(path, body)),
    patch: <T>(path: string, body?: unknown, options?: Parameters<typeof api.patch>[2]) => mutate(path, () => options ? api.patch<T>(path, body, options) : api.patch<T>(path, body)),
    del: <T>(path: string, body?: unknown, options?: Parameters<typeof api.del>[2]) => mutate(path, () => options ? api.del<T>(path, body, options) : api.del<T>(path, body)),
    postForm: <T>(path: string, form: FormData, options?: Parameters<typeof api.postForm>[2]) => mutate(path, () => options ? api.postForm<T>(path, form, options) : api.postForm<T>(path, form)),
    postBlob: (path: string, body: unknown) => mutate(path, () => api.postBlob(path, body)),
  }
}
