import {
  GRADE_LABELS,
  WORK_FORMAT_LABELS,
  type Grade,
  type Vacancy,
  type WorkFormat,
} from '@/api/types'

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

const numberFormatter = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 })

/**
 * Символ валюты по коду. Intl бросает исключение на неизвестном коде,
 * поэтому непонятный код показываем как есть — данные важнее красоты.
 */
function currencySymbol(code: string): string {
  if (!code) {
    return ''
  }
  try {
    const parts = new Intl.NumberFormat('ru-RU', {
      style: 'currency',
      currency: code,
      maximumFractionDigits: 0,
    }).formatToParts(0)
    return parts.find((part) => part.type === 'currency')?.value ?? code
  } catch {
    return code
  }
}

/**
 * Вилка из объявления в человеческом виде: «300 000 – 450 000 ₽»,
 * «от 300 000 ₽», «до 450 000 ₽». Пустая вилка даёт тире.
 */
export function formatSalary(vacancy: Pick<Vacancy, 'salary_from' | 'salary_to' | 'salary_currency'>): string {
  const { salary_from: from, salary_to: to, salary_currency: currency } = vacancy
  if (from === null && to === null) {
    return '—'
  }

  const symbol = currencySymbol(currency)
  const suffix = symbol ? ` ${symbol}` : ''

  if (from !== null && to !== null) {
    return from === to
      ? `${numberFormatter.format(from)}${suffix}`
      : `${numberFormatter.format(from)} – ${numberFormatter.format(to)}${suffix}`
  }
  if (from !== null) {
    return `от ${numberFormatter.format(from)}${suffix}`
  }
  return `до ${numberFormatter.format(to as number)}${suffix}`
}

/** Пояснение к вилке: до вычета налогов или на руки. */
export function formatSalaryGross(gross: boolean | null): string {
  if (gross === null) {
    return ''
  }
  return gross ? 'до вычета налогов' : 'на руки'
}

/** Значение для сортировки по вилке: та граница, которая известна. */
export function salarySortValue(vacancy: Pick<Vacancy, 'salary_from' | 'salary_to'>): number | undefined {
  return vacancy.salary_from ?? vacancy.salary_to ?? undefined
}

/** Формат работы человеческой подписью; неизвестное значение отдаём как есть. */
export function formatWorkFormat(value: string): string {
  if (!value) {
    return '—'
  }
  return WORK_FORMAT_LABELS[value as WorkFormat] ?? value
}

/** Заголовок карточки и первой колонки таблицы: должность важнее компании. */
export function vacancyHeading(vacancy: Pick<Vacancy, 'title' | 'company' | 'url'>): string {
  return vacancy.title || vacancy.company || formatHost(vacancy.url)
}

/** Токены с разделителями разрядов; прочерк, если провайдер их не вернул. */
export function formatTokens(value: number | null): string {
  return value === null ? '—' : numberFormatter.format(value)
}

/**
 * Оценка стоимости в долларах. Прочерк, если прайс не задан: выдуманная
 * сумма хуже отсутствующей.
 */
export function formatCost(value: number | null): string {
  if (value === null) {
    return '—'
  }
  // Запуски стоят тысячные доли доллара, поэтому знаков больше обычных двух.
  const digits = value > 0 && value < 0.01 ? 4 : 2
  return `$${value.toFixed(digits)}`
}

/** Длительность в человеческом виде: миллисекунды или секунды. */
export function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${ms} мс`
  }
  return `${(ms / 1000).toFixed(1)} с`
}

/** Человеческие подписи полей вакансии — для предпросмотра извлечения. */
export const VACANCY_FIELD_LABELS: Record<string, string> = {
  title: 'Должность',
  company: 'Компания',
  grade: 'Грейд',
  tech_tags: 'Технологии',
  opened_date: 'Дата открытия',
  salary_from: 'Зарплата от',
  salary_to: 'Зарплата до',
  salary_currency: 'Валюта',
  salary_gross: 'Указана',
  work_format: 'Формат работы',
}

/**
 * Значение поля вакансии в человеческом виде. Значения приходят из API
 * нетипизированными: поля разного вида — строки, числа, массив, дата, признак.
 */
export function formatFieldValue(field: string, value: unknown): string {
  if (value === null || value === undefined || value === '') {
    return '—'
  }

  switch (field) {
    case 'grade':
      return GRADE_LABELS[value as Grade] ?? String(value)
    case 'work_format':
      return formatWorkFormat(String(value))
    case 'opened_date':
      return formatDate(String(value))
    case 'tech_tags':
      return Array.isArray(value) ? value.join(', ') : String(value)
    case 'salary_from':
    case 'salary_to':
      return typeof value === 'number' ? numberFormatter.format(value) : String(value)
    case 'salary_gross':
      return value ? 'до вычета налогов' : 'на руки'
    default:
      return String(value)
  }
}

/** Размер файла в человеческом виде: база локального трекера редко больше мегабайтов. */
export function formatBytes(value: number): string {
  if (value < 1024) {
    return `${value} Б`
  }
  const units = ['КБ', 'МБ', 'ГБ']
  let size = value / 1024
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024
    unit += 1
  }
  // Десятая доля нужна мегабайтам, а килобайтам она только мешает.
  return `${size.toFixed(size < 10 && unit > 0 ? 1 : 0)} ${units[unit]}`
}
