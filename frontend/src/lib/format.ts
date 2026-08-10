import type { Vacancy } from '@/api/types'

const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
})

const dateTimeFormatter = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

/** Дата в формате YYYY-MM-DD приходит без времени и зоны — разбираем как локальную. */
export function formatDate(value: string | null): string {
  if (!value) {
    return '—'
  }
  const [year, month, day] = value.split('-').map(Number)
  if (!year || !month || !day) {
    return '—'
  }
  return dateFormatter.format(new Date(year, month - 1, day))
}

/** Метки времени приходят в UTC (RFC 3339) — показываем в зоне пользователя. */
export function formatDateTime(value: string | null): string {
  if (!value) {
    return '—'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? '—' : dateTimeFormatter.format(parsed)
}

export function formatHost(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

/**
 * Объясняет, откуда взялось значение is_active. Нужно там, где пользователю
 * важно понять, это решение сайта или его собственное.
 */
export function describeActivity(vacancy: Vacancy): string {
  const { manual_is_active: manual, auto_is_active: auto, activity_conflict: conflict } = vacancy

  if (manual !== null) {
    const decision = manual ? 'активна' : 'неактивна'
    if (conflict) {
      const siteSays = auto ? 'активна' : 'снята'
      return `Ваше решение: ${decision}. Последняя проверка сайта: ${siteSays}.`
    }
    return `Ваше решение: ${decision}.`
  }

  if (auto === null) {
    return 'Проверка ещё не дала результата — считаем вакансию активной.'
  }
  return auto ? 'По результату проверки: активна.' : 'По результату проверки: снята.'
}
