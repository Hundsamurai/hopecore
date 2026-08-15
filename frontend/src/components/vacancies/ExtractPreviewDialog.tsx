import { AlertTriangle, ArrowRight } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'

import { ApiError } from '@/api/client'
import type { ExtractResult } from '@/api/llm'
import { usePatchVacancy } from '@/api/vacancies'
import type { VacancyInput } from '@/api/vacancies'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { useToast } from '@/components/ui/toast'
import {
  VACANCY_FIELD_LABELS,
  formatCost,
  formatDuration,
  formatFieldValue,
  formatTokens,
} from '@/lib/format'
import { cn } from '@/lib/utils'

interface ExtractPreviewDialogProps {
  vacancyID: number
  result: ExtractResult | null
  onClose: () => void
}

/**
 * Предпросмотр извлечённых полей.
 *
 * Модель ничего не записывает сама: здесь пользователь выбирает, что применить,
 * и запись идёт обычным PATCH. По умолчанию отмечено то, что отличается
 * от текущего значения и прошло проверку.
 */
export function ExtractPreviewDialog({ vacancyID, result, onClose }: ExtractPreviewDialogProps) {
  const { show } = useToast()
  const patch = usePatchVacancy(vacancyID)

  // Порядок полей фиксирован: так таблица не пляшет между запусками.
  const rows = useMemo(() => {
    if (!result) {
      return []
    }
    return Object.keys(VACANCY_FIELD_LABELS)
      .filter((name) => name in result.fields)
      .map((name) => ({ name, field: result.fields[name] }))
  }, [result])

  /**
   * Отметки выводятся из результата, а не переносятся в состояние эффектом:
   * по умолчанию отмечено всё, что отличается, и хранить нужно только
   * то, что пользователь переключил руками. Иначе первый рендер показывал бы
   * «Применить отмеченные (0)» до срабатывания эффекта.
   */
  const [manual, setManual] = useState<{ runID: number; picks: Record<string, boolean> }>({
    runID: 0,
    picks: {},
  })

  const picks = manual.runID === result?.run_id ? manual.picks : {}
  const isChecked = (name: string, differs: boolean) => picks[name] ?? differs

  const toggle = (name: string, checked: boolean) => {
    if (!result) {
      return
    }
    setManual((prev) => ({
      runID: result.run_id,
      picks: { ...(prev.runID === result.run_id ? prev.picks : {}), [name]: checked },
    }))
  }

  const selectedNames = rows.filter(({ name, field }) => field.differs && isChecked(name, field.differs))
  const proposedCount = rows.filter(({ field }) => field.differs).length

  const apply = () => {
    if (!result || selectedNames.length === 0) {
      return
    }

    // Значения из ответа уже в формате API, поэтому передаются как есть.
    const body: Partial<VacancyInput> = {}
    for (const { name, field } of selectedNames) {
      ;(body as Record<string, unknown>)[name] = field.extracted
    }

    patch.mutate(body, {
      onSuccess: () => {
        show({
          variant: 'success',
          title: `Применено полей: ${selectedNames.length}`,
          description: selectedNames.map(({ name }) => VACANCY_FIELD_LABELS[name]).join(', '),
        })
        onClose()
      },
      onError: (error) => {
        show({
          variant: 'danger',
          title: 'Не удалось применить',
          description:
            error instanceof ApiError && Object.keys(error.fields).length > 0
              ? Object.entries(error.fields)
                  .map(([field, message]) => `${VACANCY_FIELD_LABELS[field] ?? field}: ${message}`)
                  .join('\n')
              : error.message,
        })
      },
    })
  }

  return (
    <Dialog
      open={result !== null}
      onClose={onClose}
      title="Что нашла модель"
      description={
        result ? `${result.provider} · ${result.model} · ${result.source_chars} символов страницы` : undefined
      }
      className="w-[min(48rem,calc(100vw-2rem))]"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={patch.isPending}>
            Отмена
          </Button>
          <Button
            variant="primary"
            onClick={apply}
            disabled={patch.isPending || selectedNames.length === 0}
          >
            {patch.isPending ? 'Применяем…' : `Применить отмеченные (${selectedNames.length})`}
          </Button>
        </>
      }
    >
      {result && (
        <div className="flex flex-col gap-3">
          {result.warnings.length > 0 && (
            <div
              role="status"
              className="flex items-start gap-2 rounded-md bg-warning-surface px-2.5 py-2"
            >
              <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden="true" />
              <ul className="text-xs text-foreground/90">
                {result.warnings.map((warning) => (
                  <li key={warning}>{warning}</li>
                ))}
              </ul>
            </div>
          )}

          {proposedCount === 0 && (
            <p className="rounded-md border border-dashed border-border px-2.5 py-2 text-sm text-muted">
              Модель не нашла ничего нового: все найденные значения совпадают с тем, что уже
              в карточке.
            </p>
          )}

          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full border-collapse text-left text-sm">
              <caption className="sr-only">
                Поля вакансии: текущее значение, предложение модели и отметка о применении
              </caption>
              <thead className="bg-surface">
                <tr>
                  <th scope="col" className="w-8 border-b border-border px-2 py-2" />
                  <th scope="col" className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase">
                    Поле
                  </th>
                  <th scope="col" className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase">
                    Сейчас
                  </th>
                  <th scope="col" className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase">
                    Модель предлагает
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map(({ name, field }) => (
                  <tr
                    key={name}
                    className={cn(
                      'border-b border-border/60 last:border-0',
                      !field.differs && 'text-muted',
                    )}
                  >
                    <td className="px-2 py-2 align-top">
                      <input
                        type="checkbox"
                        checked={field.differs && isChecked(name, field.differs)}
                        // Отмечать нечего там, где значение не отличается
                        // или было отброшено при проверке.
                        disabled={!field.differs}
                        onChange={(event) => toggle(name, event.target.checked)}
                        aria-label={`Применить поле «${VACANCY_FIELD_LABELS[name]}»`}
                        className="size-4 accent-primary disabled:opacity-40"
                      />
                    </td>
                    <td className="px-3 py-2 align-top">{VACANCY_FIELD_LABELS[name]}</td>
                    <td className="px-3 py-2 align-top">{formatFieldValue(name, field.current)}</td>
                    <td className="px-3 py-2 align-top">
                      {field.differs ? (
                        <span className="inline-flex items-start gap-1.5 font-medium text-foreground">
                          <ArrowRight className="mt-0.5 size-3.5 shrink-0 text-primary" aria-hidden="true" />
                          {formatFieldValue(name, field.extracted)}
                        </span>
                      ) : (
                        <span>{field.has_value ? 'без изменений' : '—'}</span>
                      )}
                      {field.note && (
                        <span className="mt-0.5 block text-xs text-warning">{field.note}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <p className="text-xs text-muted">
            Токены: {formatTokens(result.usage.input_tokens)} вход /{' '}
            {formatTokens(result.usage.output_tokens)} выход · оценка{' '}
            {formatCost(result.usage.cost_estimate)} · {formatDuration(result.usage.duration_ms)}
            {result.usage.attempts > 1 && ` · попыток: ${result.usage.attempts}`} ·{' '}
            <Link
              to="/llm/runs"
              onClick={onClose}
              className="text-primary underline-offset-2 hover:underline"
            >
              запуск №{result.run_id}
            </Link>
          </p>
        </div>
      )}
    </Dialog>
  )
}
