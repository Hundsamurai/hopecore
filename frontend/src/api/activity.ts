import { useMutation, useQueryClient } from '@tanstack/react-query'

import { apiRequest } from './client'
import type { CheckSummary, Vacancy } from './types'
import { vacancyKeys } from './vacancies'

/**
 * Проверка активности одной вакансии. Запускается только по кнопке —
 * крона в MVP нет (п. 3 ТЗ), все обращения к чужим сайтам инициирует пользователь.
 */
export function useCheckVacancy(id: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => apiRequest<Vacancy>(`/vacancies/${id}/check`, { method: 'POST' }),
    onSuccess: (vacancy) => {
      queryClient.setQueryData(vacancyKeys.detail(id), vacancy)
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}

/** Массовая проверка: возвращает сводку, которую UI показывает уведомлением. */
export function useCheckAllVacancies() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: () => apiRequest<CheckSummary>('/vacancies/check', { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}

/**
 * Ручной статус активности. null снимает override: вакансия снова живёт
 * по результату авто-проверки.
 */
export function useSetActivity(id: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (manualIsActive: boolean | null) =>
      apiRequest<Vacancy>(`/vacancies/${id}/activity`, {
        method: 'PUT',
        body: { manual_is_active: manualIsActive },
      }),
    onSuccess: (vacancy) => {
      queryClient.setQueryData(vacancyKeys.detail(id), vacancy)
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}

/** Человекочитаемая сводка массовой проверки для уведомления. */
export function describeSummary(summary: CheckSummary): string {
  const parts = [`опрошено: ${summary.checked}`]

  if (summary.became_inactive > 0) {
    parts.push(`снято с публикации: ${summary.became_inactive}`)
  }
  if (summary.unknown > 0) {
    parts.push(`ответ неинформативен: ${summary.unknown}`)
  }
  if (summary.failed > 0) {
    parts.push(`без ответа: ${summary.failed}`)
  }
  if (summary.skipped > 0) {
    parts.push(`пропущено как закрытые вручную: ${summary.skipped}`)
  }

  return parts.join(', ')
}
