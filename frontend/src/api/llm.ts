import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { apiRequest } from './client'

/** Провайдер языковой модели, доступный по конфигурации (ключ + модели). */
export interface LLMProvider {
  id: string
  label: string
  models: string[]
  default_model: string
  /** Задана ли цена: без неё показываются только токены, без суммы. */
  price_known: boolean
}

/** Итоги запуска в журнале. */
export type LLMRunStatus =
  | 'ok'
  | 'fetch_error'
  | 'provider_error'
  | 'invalid_json'
  | 'timeout'

export interface LLMRun {
  id: number
  created_at: string
  purpose: string

  /** null, если вакансию удалили: запись о трате переживает карточку. */
  vacancy_id: number | null
  vacancy_title: string

  provider: string
  model: string
  prompt_version: string

  source_url: string
  source_chars: number

  status: LLMRunStatus
  input_tokens: number | null
  output_tokens: number | null
  cost_estimate: number | null
  /** Больше одной попытки означает, что первый ответ не разобрался. */
  attempts: number
  duration_ms: number
  error: string
}

/** Карточка запуска: то же плюс сырой ответ модели. */
export interface LLMRunDetail extends LLMRun {
  response_json: string
}

/** Суммарный расход по всему журналу, а не по текущему фильтру. */
export interface LLMUsage {
  runs: number
  input_tokens: number
  output_tokens: number
  cost_estimate: number | null
  /** По скольким запускам стоимость известна. */
  priced_runs: number
}

export interface LLMRunList {
  items: LLMRun[]
  /** Сколько записей подходит под фильтр, а не сколько отдано. */
  count: number
  usage: LLMUsage
}

export interface LLMRunsParams {
  limit?: number
  vacancyId?: number
}

export const llmKeys = {
  providers: ['llm', 'providers'] as const,
  runs: (params: LLMRunsParams) => ['llm', 'runs', params] as const,
  run: (id: number) => ['llm', 'run', id] as const,
}

export function useLLMProviders() {
  return useQuery({
    queryKey: llmKeys.providers,
    queryFn: ({ signal }) =>
      apiRequest<{ items: LLMProvider[] }>('/llm/providers', { signal }),
    select: (data) => data.items,
    // Провайдеры задаются переменными окружения и меняются только с перезапуском.
    staleTime: Infinity,
  })
}

export function useLLMRuns(params: LLMRunsParams = {}) {
  return useQuery({
    queryKey: llmKeys.runs(params),
    queryFn: ({ signal }) =>
      apiRequest<LLMRunList>('/llm/runs', {
        signal,
        query: { limit: params.limit, vacancy_id: params.vacancyId },
      }),
  })
}

export function useLLMRun(id: number | null) {
  return useQuery({
    queryKey: llmKeys.run(id ?? 0),
    queryFn: ({ signal }) => apiRequest<LLMRunDetail>(`/llm/runs/${id}`, { signal }),
    enabled: id !== null && id > 0,
  })
}

/** Человеческие подписи статусов. */
export const RUN_STATUS_LABELS: Record<LLMRunStatus, string> = {
  ok: 'успех',
  fetch_error: 'страница не прочитана',
  provider_error: 'ошибка провайдера',
  invalid_json: 'ответ не разобран',
  timeout: 'таймаут',
}

/** Одно поле в предпросмотре извлечения. */
export interface ExtractedField {
  /** Что предлагает модель; null — не нашла либо значение отбросили. */
  extracted: unknown
  /** Что сейчас в карточке. */
  current: unknown
  /** Модель дала пригодное значение. */
  has_value: boolean
  /** Отличается от текущего: такие поля и отмечаются галочками. */
  differs: boolean
  /** Почему значение отброшено, если это случилось. */
  note?: string
}

export interface ExtractUsage {
  input_tokens: number
  output_tokens: number
  cost_estimate: number | null
  attempts: number
  duration_ms: number
}

export interface ExtractResult {
  run_id: number
  provider: string
  model: string
  source_url: string
  source_chars: number
  /** Про страницу целиком: мало текста, антибот, обрезка по лимиту. */
  warnings: string[]
  fields: Record<string, ExtractedField>
  usage: ExtractUsage
}

export interface ExtractParams {
  provider: string
  model?: string
}

/**
 * Запуск извлечения. В вакансию ничего не пишет: результат уходит
 * на предпросмотр, а запись идёт обычным PATCH после подтверждения.
 */
export function useExtractVacancy(vacancyID: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (params: ExtractParams) =>
      apiRequest<ExtractResult>(`/vacancies/${vacancyID}/extract`, {
        method: 'POST',
        body: { provider: params.provider, model: params.model ?? '' },
      }),
    onSettled: () => {
      // Журнал обновляем в любом случае: неудачные запуски тоже туда попадают.
      void queryClient.invalidateQueries({ queryKey: ['llm', 'runs'] })
    },
  })
}
