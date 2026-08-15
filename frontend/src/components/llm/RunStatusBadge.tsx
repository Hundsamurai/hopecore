import { RUN_STATUS_LABELS, type LLMRunStatus } from '@/api/llm'
import { Badge } from '@/components/ui/badge'

/**
 * Статус запуска цветом и словами. Только цвет различать нельзя, поэтому
 * подпись всегда рядом.
 */
export function RunStatusBadge({ status, title }: { status: LLMRunStatus; title?: string }) {
  const variant = status === 'ok' ? 'success' : status === 'timeout' ? 'warning' : 'danger'

  return (
    <Badge variant={variant} title={title}>
      {RUN_STATUS_LABELS[status] ?? status}
    </Badge>
  )
}
