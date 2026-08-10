import { Hand, TriangleAlert } from 'lucide-react'

import type { Vacancy } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { describeActivity, formatDateTime } from '@/lib/format'

interface ActivityBadgeProps {
  vacancy: Vacancy
}

/**
 * Бейдж активности с двумя признаками происхождения значения:
 *   — рука: значение задано вручную, авто-проверка его не переопределит;
 *   — треугольник: ручное решение расходится с результатом последней проверки.
 *
 * Иконки не заменяют текст, а дополняют его, и обе имеют доступное описание:
 * различать состояния только цветом нельзя.
 */
export function ActivityBadge({ vacancy }: ActivityBadgeProps) {
  const isManual = vacancy.manual_is_active !== null
  const title = [
    describeActivity(vacancy),
    vacancy.last_checked_at ? `Проверено: ${formatDateTime(vacancy.last_checked_at)}` : 'Проверок ещё не было',
    vacancy.last_check_error && `Ошибка проверки: ${vacancy.last_check_error}`,
  ]
    .filter(Boolean)
    .join('\n')

  return (
    <span className="inline-flex items-center gap-1" title={title}>
      <Badge variant={vacancy.is_active ? 'success' : 'danger'}>
        {vacancy.is_active ? 'активна' : 'снята'}
      </Badge>

      {isManual && (
        <>
          <Hand className="size-3.5 text-muted" aria-hidden="true" />
          <span className="sr-only">задано вручную</span>
        </>
      )}

      {vacancy.activity_conflict && (
        <>
          <TriangleAlert className="size-3.5 text-warning" aria-hidden="true" />
          <span className="sr-only">сайт сообщает обратное</span>
        </>
      )}
    </span>
  )
}
