import { QueryClient } from '@tanstack/react-query'

import { ApiError } from '@/api/client'

/**
 * Настройки под локальный однопользовательский инструмент: данные меняются
 * только из этого же окна, поэтому агрессивный refetch не нужен.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        // Ошибки клиента повторять бессмысленно: 404 и 400 сами не исправятся.
        if (error instanceof ApiError && error.status < 500) {
          return false
        }
        return failureCount < 2
      },
    },
    mutations: {
      retry: false,
    },
  },
})
