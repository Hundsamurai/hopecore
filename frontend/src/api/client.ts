/**
 * Единая точка обращения к API.
 *
 * Базовый путь относительный: в проде запрос идёт через nginx, который проксирует
 * /api на backend, поэтому фронт не знает его адреса и CORS не нужен.
 */
const API_BASE = '/api'

/** Формат ошибки из errorBody на backend. */
interface ApiErrorBody {
  error: {
    code: string
    message: string
    fields?: Record<string, string>
  }
}

/**
 * ApiError несёт код и ошибки по полям, чтобы форма могла разложить их
 * по инпутам, а не показывать одну строку на весь диалог.
 */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly fields: Record<string, string>

  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
  }

  /** Ошибки валидации имеет смысл показывать рядом с полями формы. */
  get isValidation(): boolean {
    return this.code === 'validation_failed' || this.code === 'invalid_json'
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  query?: Record<string, string | number | boolean | undefined>
}

function buildUrl(path: string, query?: RequestOptions['query']): string {
  const url = `${API_BASE}${path}`
  if (!query) {
    return url
  }

  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined) {
      params.set(key, String(value))
    }
  }

  const search = params.toString()
  return search ? `${url}?${search}` : url
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, signal, query } = options

  const response = await fetch(buildUrl(path, query), {
    method,
    signal,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (response.status === 204) {
    return undefined as T
  }

  const raw = await response.text()

  if (!response.ok) {
    throw toApiError(response.status, raw)
  }

  if (!raw) {
    return undefined as T
  }

  return JSON.parse(raw) as T
}

function toApiError(status: number, raw: string): ApiError {
  try {
    const parsed = JSON.parse(raw) as ApiErrorBody
    if (parsed.error?.code) {
      return new ApiError(status, parsed.error.code, parsed.error.message, parsed.error.fields ?? {})
    }
  } catch {
    // Тело не в нашем формате — например, ошибка от nginx. Ниже общий текст.
  }
  return new ApiError(status, 'unknown', `Сервер ответил ${status}`)
}
