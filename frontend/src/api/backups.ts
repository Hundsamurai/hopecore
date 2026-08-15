import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiRequest } from './client'

/** Файл резервной копии базы. */
export interface Backup {
  name: string
  size_bytes: number
  created_at: string
  /**
   * Копия снята приложением автоматически перед восстановлением,
   * а не по кнопке. Именно из неё откатывают ошибочное восстановление.
   */
  automatic: boolean
}

export interface BackupList {
  items: Backup[]
  /** Каталог с файлами: копии можно забрать с диска руками. */
  dir: string
  total_bytes: number
}

/** Ответ на восстановление: откуда восстановились и чем можно откатиться. */
export interface RestoreResult {
  restored: string
  safety_backup: string
}

export const backupKeys = {
  list: ['backups'] as const,
}

export function useBackups() {
  return useQuery({
    queryKey: backupKeys.list,
    queryFn: ({ signal }) => apiRequest<BackupList>('/backups', { signal }),
  })
}

export function useCreateBackup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => apiRequest<Backup>('/backups', { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupKeys.list })
    },
  })
}

export function useRestoreBackup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (name: string) =>
      apiRequest<RestoreResult>(`/backups/${encodeURIComponent(name)}/restore`, {
        method: 'POST',
      }),
    onSuccess: () => {
      // Восстановление меняет все таблицы сразу, поэтому сбрасывается весь кэш,
      // а не отдельные ключи: на экране не должно остаться данных из прошлого состояния.
      void queryClient.invalidateQueries()
    },
  })
}

export function useDeleteBackup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (name: string) =>
      apiRequest<void>(`/backups/${encodeURIComponent(name)}`, { method: 'DELETE' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupKeys.list })
    },
  })
}
