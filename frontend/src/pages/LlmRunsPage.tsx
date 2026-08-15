import { AlertCircle } from 'lucide-react'
import { useState } from 'react'

import { useLLMProviders, useLLMRuns } from '@/api/llm'
import { Spinner } from '@/components/ui/spinner'
import { RunDetailDialog } from '@/components/llm/RunDetailDialog'
import { RunStatusBadge } from '@/components/llm/RunStatusBadge'
import { formatCost, formatDateTime, formatDuration, formatHost, formatTokens } from '@/lib/format'

/** Сколько записей запрашиваем: журнал локального инструмента невелик. */
const PAGE_SIZE = 50

/**
 * Экран журнала запусков модели. Отвечает на два вопроса: куда уходит квота
 * и чем заполнена конкретная вакансия.
 */
export function LlmRunsPage() {
  const [openRunID, setOpenRunID] = useState<number | null>(null)

  const { data, isPending, isError, error } = useLLMRuns({ limit: PAGE_SIZE })
  const { data: providers } = useLLMProviders()

  const usage = data?.usage

  return (
    <div className="mx-auto max-w-6xl p-6">
      <header className="mb-4">
        <h1 className="text-xl font-semibold tracking-tight">Запуски LLM</h1>
        <p className="mt-1 text-sm text-muted">
          Каждое обращение к модели инициируете вы: ничего не запускается по расписанию.
        </p>
      </header>

      {providers?.length === 0 && (
        <p className="mb-4 rounded-lg border border-border bg-surface px-3 py-2 text-sm text-muted">
          Провайдеры не настроены: добавьте ключ в <code>.env</code>, чтобы заполнять карточки
          через модель.
        </p>
      )}

      {usage && usage.runs > 0 && (
        <section
          aria-label="Расход по всему журналу"
          className="mb-4 grid gap-3 rounded-lg border border-border bg-surface p-4 sm:grid-cols-4"
        >
          <Total label="Запусков" value={String(usage.runs)} />
          <Total label="Входных токенов" value={formatTokens(usage.input_tokens)} />
          <Total label="Выходных токенов" value={formatTokens(usage.output_tokens)} />
          <Total
            label="Оценка стоимости"
            value={formatCost(usage.cost_estimate)}
            hint={
              usage.cost_estimate === null
                ? 'цены не заданы в настройках'
                : usage.priced_runs < usage.runs
                  ? `известна по ${usage.priced_runs} из ${usage.runs} запусков`
                  : undefined
            }
          />
        </section>
      )}

      {isPending && (
        <div className="rounded-lg border border-border bg-surface p-6">
          <Spinner label="Загружаем журнал" />
        </div>
      )}

      {isError && (
        <div className="flex items-start gap-3 rounded-lg border border-danger/40 bg-danger-surface p-4">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">Не удалось загрузить журнал</p>
            <p className="mt-1 text-sm text-muted">{error.message}</p>
          </div>
        </div>
      )}

      {data?.items.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-10 text-center">
          <p className="text-sm font-medium">Модель ещё не запускали</p>
          <p className="mx-auto mt-1 max-w-md text-sm text-muted">
            Заполнение карточки через модель запускается из вакансии. Здесь будет видно, чем
            и когда заполнено каждое поле и сколько это стоило.
          </p>
        </div>
      )}

      {data && data.items.length > 0 && (
        <>
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full border-collapse text-left">
              <caption className="sr-only">
                Журнал запусков модели: время, вакансия, провайдер и модель, итог, токены,
                оценка стоимости и длительность
              </caption>
              <thead className="bg-surface">
                <tr>
                  {['Когда', 'Вакансия', 'Модель', 'Итог', 'Токены', 'Стоимость', 'Время'].map(
                    (title) => (
                      <th
                        key={title}
                        scope="col"
                        className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase"
                      >
                        {title}
                      </th>
                    ),
                  )}
                </tr>
              </thead>
              <tbody>
                {data.items.map((run) => (
                  <tr
                    key={run.id}
                    onClick={() => setOpenRunID(run.id)}
                    className="cursor-pointer border-b border-border/60 transition-colors last:border-0 hover:bg-surface"
                  >
                    <td className="px-3 py-2 text-sm whitespace-nowrap text-muted">
                      {formatDateTime(run.created_at)}
                    </td>
                    <td className="max-w-64 px-3 py-2">
                      {/* Клавиатурный путь к деталям — эта кнопка, а не строка. */}
                      <button
                        type="button"
                        onClick={() => setOpenRunID(run.id)}
                        className="truncate text-left text-sm underline-offset-2 hover:underline"
                      >
                        {run.vacancy_title || formatHost(run.source_url)}
                      </button>
                      {!run.vacancy_id && (
                        <span className="block text-xs text-muted">вакансия удалена</span>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      <span className="text-sm whitespace-nowrap">{run.model}</span>
                      <span className="block text-xs text-muted">{run.provider}</span>
                    </td>
                    <td className="px-3 py-2">
                      <RunStatusBadge status={run.status} title={run.error || undefined} />
                      {run.attempts > 1 && (
                        <span className="block text-xs text-muted">
                          попыток: {run.attempts}
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-sm whitespace-nowrap">
                      {formatTokens(run.input_tokens)} / {formatTokens(run.output_tokens)}
                    </td>
                    <td className="px-3 py-2 text-sm whitespace-nowrap">
                      {formatCost(run.cost_estimate)}
                    </td>
                    <td className="px-3 py-2 text-sm whitespace-nowrap text-muted">
                      {formatDuration(run.duration_ms)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {data.count > data.items.length && (
            <p className="mt-2 text-xs text-muted">
              Показаны последние {data.items.length} из {data.count} запусков.
            </p>
          )}
        </>
      )}

      <RunDetailDialog runID={openRunID} onClose={() => setOpenRunID(null)} />
    </div>
  )
}

function Total({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div>
      <p className="text-xs text-muted">{label}</p>
      <p className="mt-0.5 text-lg font-semibold tabular-nums">{value}</p>
      {hint && <p className="mt-0.5 text-xs text-muted/70">{hint}</p>}
    </div>
  )
}


