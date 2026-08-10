import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiRequest } from './client'
import type { Vacancy, VacancyList, VacancyListParams } from './types'

/**
 * Ключи запросов собраны в одном месте: инвалидация после мутаций должна
 * ссылаться на те же ключи, что и чтение.
 */
export const vacancyKeys = {
  all: ['vacancies'] as const,
  list: (params: VacancyListParams) => ['vacancies', 'list', params] as const,
  detail: (id: number) => ['vacancies', 'detail', id] as const,
}

/** Поля вакансии, которые заполняет пользователь (Режим 1 из п. 3 ТЗ). */
export interface VacancyInput {
  url: string
  company: string
  grade: string
  tech_tags: string[]
  opened_date: string | null
}

export function fetchVacancies(
  params: VacancyListParams,
  signal?: AbortSignal,
): Promise<VacancyList> {
  return apiRequest<VacancyList>('/vacancies', {
    signal,
    query: {
      include_inactive: params.includeInactive ? true : undefined,
      sort: params.sort,
      order: params.order,
    },
  })
}

export function fetchVacancy(id: number, signal?: AbortSignal): Promise<Vacancy> {
  return apiRequest<Vacancy>(`/vacancies/${id}`, { signal })
}

export function useVacancies(params: VacancyListParams = {}) {
  return useQuery({
    queryKey: vacancyKeys.list(params),
    queryFn: ({ signal }) => fetchVacancies(params, signal),
    select: (data) => data.items,
  })
}

export function useVacancy(id: number) {
  return useQuery({
    queryKey: vacancyKeys.detail(id),
    queryFn: ({ signal }) => fetchVacancy(id, signal),
    enabled: Number.isInteger(id) && id > 0,
  })
}

export function useCreateVacancy() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: VacancyInput) =>
      apiRequest<Vacancy>('/vacancies', { method: 'POST', body: input }),
    onSuccess: () => {
      // Инвалидируем всё поддерево vacancies: список зависит от фильтра
      // и сортировки, перечислять все варианты ключей бессмысленно.
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}

export function useUpdateVacancy(id: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: VacancyInput) =>
      apiRequest<Vacancy>(`/vacancies/${id}`, { method: 'PATCH', body: input }),
    onSuccess: (vacancy) => {
      // Свежая карточка уже пришла в ответе — кладём её в кеш сразу,
      // чтобы экран не мигал спиннером после сохранения.
      queryClient.setQueryData(vacancyKeys.detail(id), vacancy)
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}
