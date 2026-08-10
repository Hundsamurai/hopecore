import { AlertCircle, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import { describeSummary, useCheckAllVacancies } from '@/api/activity'
import { useVacancies } from '@/api/vacancies'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Spinner } from '@/components/ui/spinner'
import { useToast } from '@/components/ui/toast'
import { VacancyFormDialog } from '@/components/vacancies/VacancyFormDialog'
import { VacancyTable } from '@/components/vacancies/VacancyTable'
import { cn } from '@/lib/utils'

const INACTIVE_PARAM = 'inactive'

/**
 * Таблица вакансий — основной экран (п. 6 ТЗ).
 *
 * Флаг «показать неактивные» живёт в URL, а не в состоянии компонента:
 * так его видно в адресной строке и можно вернуться к тому же виду по ссылке
 * или кнопкой «назад» из карточки.
 */
export function VacanciesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const includeInactive = searchParams.get(INACTIVE_PARAM) === '1'
  const [formOpen, setFormOpen] = useState(false)

  const { show } = useToast()
  const checkAll = useCheckAllVacancies()

  const runCheckAll = () => {
    checkAll.mutate(undefined, {
      onSuccess: (summary) => {
        if (summary.checked === 0 && summary.skipped === 0) {
          show({ variant: 'info', title: 'Проверять нечего', description: 'В базе нет вакансий.' })
          return
        }
        show({
          variant: summary.became_inactive > 0 ? 'danger' : 'success',
          title: 'Проверка завершена',
          description: describeSummary(summary),
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

  const { data: vacancies, isPending, isError, error, isFetching } = useVacancies({ includeInactive })

  const setIncludeInactive = (next: boolean) => {
    setSearchParams(
      (params) => {
        if (next) {
          params.set(INACTIVE_PARAM, '1')
        } else {
          params.delete(INACTIVE_PARAM)
        }
        return params
      },
      { replace: true },
    )
  }

  return (
    <div className="mx-auto max-w-6xl p-6">
      <header className="mb-4 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Вакансии</h1>
          <p className="mt-1 text-sm text-muted">
            Сортировка по любому столбцу. По умолчанию — по дате изменения.
          </p>
        </div>

        <div className="flex items-center gap-4">
          {/* Индикатор фонового обновления: список уже показан, но данные перезапрашиваются. */}
          {isFetching && !isPending && <Spinner label="Обновляем список" />}
          <Switch
            checked={includeInactive}
            onChange={setIncludeInactive}
            label="Показать неактивные"
          />
          {vacancies && (
            <span className="text-sm text-muted" aria-live="polite">
              {vacancies.length} шт.
            </span>
          )}

          <Button onClick={runCheckAll} disabled={checkAll.isPending}>
            <RefreshCw
              className={cn('size-4', checkAll.isPending && 'animate-spin')}
              aria-hidden="true"
            />
            {checkAll.isPending ? 'Опрашиваем…' : 'Опросить все'}
          </Button>

          <Button variant="primary" onClick={() => setFormOpen(true)}>
            <Plus className="size-4" aria-hidden="true" />
            Добавить
          </Button>
        </div>
      </header>

      <VacancyFormDialog open={formOpen} onClose={() => setFormOpen(false)} />

      {isPending && (
        <div className="rounded-lg border border-border bg-surface p-6">
          <Spinner label="Загружаем вакансии" />
        </div>
      )}

      {isError && (
        <div className="flex items-start gap-3 rounded-lg border border-danger/40 bg-danger-surface p-4">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">Не удалось загрузить вакансии</p>
            <p className="mt-1 text-sm text-muted">{error.message}</p>
          </div>
        </div>
      )}

      {vacancies?.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-10 text-center">
          <p className="text-sm font-medium">
            {includeInactive ? 'Пока ни одной вакансии' : 'Активных вакансий нет'}
          </p>
          <p className="mx-auto mt-1 max-w-md text-sm text-muted">
            {includeInactive
              ? 'Добавьте первую вакансию по ссылке — кнопка «Добавить» справа сверху.'
              : 'Возможно, все вакансии помечены снятыми — включите «Показать неактивные».'}
          </p>
        </div>
      )}

      {vacancies && vacancies.length > 0 && <VacancyTable vacancies={vacancies} />}
    </div>
  )
}
