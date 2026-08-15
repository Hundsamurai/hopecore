import { ExternalLink } from 'lucide-react'
import { Link } from 'react-router-dom'

import { useLLMRun } from '@/api/llm'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { formatCost, formatDateTime, formatDuration, formatTokens } from '@/lib/format'

import { RunStatusBadge } from './RunStatusBadge'

interface RunDetailDialogProps {
  runID: number | null
  onClose: () => void
}

/**
 * Карточка запуска с сырым ответом модели. Смысл экрана — увидеть, что именно
 * сказала модель, когда результат выглядит странно.
 */
export function RunDetailDialog({ runID, onClose }: RunDetailDialogProps) {
  const { data: run, isPending, isError, error } = useLLMRun(runID)

  return (
    <Dialog
      open={runID !== null}
      onClose={onClose}
      title={`Запуск №${runID ?? ''}`}
      description={run ? `${run.provider} · ${run.model}` : undefined}
      className="w-[min(44rem,calc(100vw-2rem))]"
      footer={
        <Button variant="ghost" onClick={onClose}>
          Закрыть
        </Button>
      }
    >
      {isPending && <Spinner label="Загружаем запуск" />}

      {isError && (
        <p role="alert" className="text-sm text-danger">
          {error.message}
        </p>
      )}

      {run && (
        <div className="flex flex-col gap-4">
          <dl className="grid gap-3 sm:grid-cols-2">
            <Field label="Итог">
              <RunStatusBadge status={run.status} />
            </Field>
            <Field label="Когда">{formatDateTime(run.created_at)}</Field>
            <Field label="Вакансия">
              {run.vacancy_id ? (
                <Link
                  to={`/vacancies/${run.vacancy_id}`}
                  onClick={onClose}
                  className="text-primary underline-offset-2 hover:underline"
                >
                  {run.vacancy_title || `№${run.vacancy_id}`}
                </Link>
              ) : (
                // Вакансию удалили, а запись о трате осталась.
                <span className="text-muted">удалена</span>
              )}
            </Field>
            <Field label="Страница">
              <a
                href={run.source_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 break-all text-muted underline-offset-2 hover:text-foreground hover:underline"
              >
                {run.source_url}
                <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
              </a>
            </Field>
            <Field label="Символов со страницы">{run.source_chars}</Field>
            <Field label="Токены">
              {formatTokens(run.input_tokens)} вход / {formatTokens(run.output_tokens)} выход
            </Field>
            <Field label="Оценка стоимости">{formatCost(run.cost_estimate)}</Field>
            <Field label="Длительность">{formatDuration(run.duration_ms)}</Field>
            <Field label="Попыток">
              {run.attempts}
              {run.attempts > 1 && (
                <span className="ml-1.5 text-xs text-muted">
                  первый ответ не разобрался
                </span>
              )}
            </Field>
            <Field label="Версия промпта">{run.prompt_version || '—'}</Field>
          </dl>

          {run.error && (
            <div className="rounded-md bg-danger-surface px-2.5 py-2">
              <p className="text-xs text-muted">Причина</p>
              <p className="mt-0.5 text-sm text-danger">{run.error}</p>
            </div>
          )}

          <div>
            <p className="text-xs text-muted">Ответ модели как есть</p>
            <pre className="mt-1 max-h-64 overflow-auto rounded-md border border-border bg-background p-2.5 text-xs whitespace-pre-wrap">
              {formatJSON(run.response_json) || '— пусто: до модели дело не дошло'}
            </pre>
          </div>
        </div>
      )}
    </Dialog>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-muted">{label}</dt>
      <dd className="mt-0.5 text-sm">{children}</dd>
    </div>
  )
}

/** Разбираем и печатаем с отступами; если не JSON — показываем как есть. */
function formatJSON(raw: string): string {
  if (!raw.trim()) {
    return ''
  }
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}
