import { RefreshCw } from 'lucide-react'
import { useId } from 'react'

import { useCheckVacancy, useSetActivity } from '@/api/activity'
import type { Vacancy } from '@/api/types'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/toast'
import { describeActivity, formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

interface ActivityPanelProps {
  vacancy: Vacancy
}

/** Три состояния переключателя: авто и два ручных решения. */
const OPTIONS: Array<{ value: 'auto' | 'active' | 'inactive'; label: string; hint: string }> = [
  { value: 'auto', label: 'По проверке', hint: 'Значение берётся из результата опроса сайта' },
  { value: 'active', label: 'Активна', hint: 'Ваше решение: проверка его не переопределит' },
  { value: 'inactive', label: 'Снята', hint: 'Ваше решение: массовый опрос такие вакансии пропускает' },
]

function currentOption(vacancy: Vacancy): 'auto' | 'active' | 'inactive' {
  if (vacancy.manual_is_active === null) {
    return 'auto'
  }
  return vacancy.manual_is_active ? 'active' : 'inactive'
}

export function ActivityPanel({ vacancy }: ActivityPanelProps) {
  const groupId = useId()
  const { show } = useToast()

  const check = useCheckVacancy(vacancy.id)
  const setActivity = useSetActivity(vacancy.id)

  const selected = currentOption(vacancy)

  const runCheck = () => {
    check.mutate(undefined, {
      onSuccess: (fresh) => {
        const code = fresh.last_check_code === null ? 'без ответа' : `код ${fresh.last_check_code}`

        if (fresh.last_check_error) {
          show({
            variant: 'info',
            title: 'Проверка не дала результата',
            description: `${fresh.last_check_error}\nПрежнее значение активности сохранено.`,
          })
          return
        }

        show({
          variant: fresh.auto_is_active ? 'success' : 'danger',
          title: fresh.auto_is_active ? `Вакансия на месте (${code})` : `Вакансия снята (${code})`,
          description:
            vacancy.manual_is_active !== null
              ? 'Ручной статус оставлен без изменений — он важнее результата проверки.'
              : undefined,
        })
      },
      onError: (error) => {
        show({
          variant: 'danger',
          title: 'Не удалось выполнить проверку',
          description: error instanceof Error ? error.message : String(error),
        })
      },
    })
  }

  const changeOption = (option: 'auto' | 'active' | 'inactive') => {
    const manual = option === 'auto' ? null : option === 'active'

    setActivity.mutate(manual, {
      onError: (error) => {
        show({
          variant: 'danger',
          title: 'Не удалось изменить статус',
          description: error instanceof Error ? error.message : String(error),
        })
      },
    })
  }

  return (
    <section className="mt-4 rounded-lg border border-border bg-surface p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-medium">Активность</h2>
          <p className="mt-1 text-sm text-muted">{describeActivity(vacancy)}</p>
        </div>

        <Button onClick={runCheck} disabled={check.isPending}>
          <RefreshCw
            className={cn('size-3.5', check.isPending && 'animate-spin')}
            aria-hidden="true"
          />
          {check.isPending ? 'Опрашиваем…' : 'Опросить сейчас'}
        </Button>
      </div>

      <dl className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-xs">
        <div className="flex gap-1">
          <dt className="text-muted">Последняя проверка:</dt>
          <dd>{formatDateTime(vacancy.last_checked_at)}</dd>
        </div>
        <div className="flex gap-1">
          <dt className="text-muted">Код ответа:</dt>
          <dd>{vacancy.last_check_code ?? '—'}</dd>
        </div>
        {vacancy.last_check_error && (
          <div className="flex gap-1">
            <dt className="text-muted">Причина:</dt>
            <dd className="text-warning">{vacancy.last_check_error}</dd>
          </div>
        )}
      </dl>

      <fieldset className="mt-4" disabled={setActivity.isPending}>
        <legend id={groupId} className="text-xs text-muted">
          Статус активности
        </legend>

        <div role="radiogroup" aria-labelledby={groupId} className="mt-1.5 flex flex-wrap gap-2">
          {OPTIONS.map((option) => (
            <label
              key={option.value}
              title={option.hint}
              className={cn(
                'cursor-pointer rounded-md border px-2.5 py-1.5 text-sm transition-colors',
                selected === option.value
                  ? 'border-primary bg-primary/15 text-foreground'
                  : 'border-border text-muted hover:bg-surface-hover hover:text-foreground',
              )}
            >
              <input
                type="radio"
                name={`activity-${vacancy.id}`}
                value={option.value}
                checked={selected === option.value}
                onChange={() => changeOption(option.value)}
                className="sr-only"
              />
              {option.label}
            </label>
          ))}
        </div>

        <p className="mt-2 text-xs text-muted/70">
          {OPTIONS.find((option) => option.value === selected)?.hint}
        </p>
      </fieldset>
    </section>
  )
}
