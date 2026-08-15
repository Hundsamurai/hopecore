import { AlertCircle, ArrowLeft, ExternalLink, Pencil } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'

import { GRADE_LABELS, type Grade } from '@/api/types'
import { useVacancy } from '@/api/vacancies'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { ActivityBadge } from '@/components/vacancies/ActivityBadge'
import { ActivityPanel } from '@/components/vacancies/ActivityPanel'
import { CandidateStatusForm } from '@/components/vacancies/CandidateStatusForm'
import { StagePlaceholder } from '@/components/vacancies/StagePlaceholder'
import { VacancyFormDialog } from '@/components/vacancies/VacancyFormDialog'
import {
  formatDate,
  formatDateTime,
  formatSalary,
  formatSalaryGross,
  formatWorkFormat,
  vacancyHeading,
} from '@/lib/format'

/**
 * Карточка вакансии. На этом шаге показывает данные вакансии — нужна, чтобы
 * клик по строке таблицы вёл в осмысленное место. Форма статуса кандидата
 * и заглушки Этапа 3 добавляются в Task 10.
 */
export function VacancyPage() {
  const { id } = useParams()
  const vacancyId = Number(id)

  const [editOpen, setEditOpen] = useState(false)

  const { data: vacancy, isPending, isError, error } = useVacancy(vacancyId)

  if (!Number.isInteger(vacancyId) || vacancyId <= 0) {
    return <NotFound />
  }

  return (
    <div className="mx-auto max-w-3xl p-6">
      <Link
        to="/vacancies"
        className="inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-4" aria-hidden="true" />
        К списку
      </Link>

      {isPending && (
        <div className="mt-4 rounded-lg border border-border bg-surface p-6">
          <Spinner label="Загружаем вакансию" />
        </div>
      )}

      {isError && (
        <div className="mt-4 flex items-start gap-3 rounded-lg border border-danger/40 bg-danger-surface p-4">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">Не удалось загрузить вакансию</p>
            <p className="mt-1 text-sm text-muted">{error.message}</p>
          </div>
        </div>
      )}

      {vacancy && (
        <article className="mt-4">
          <header className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <h1 className="text-xl font-semibold tracking-tight">{vacancyHeading(vacancy)}</h1>
              {/* Компания под должностью: заголовок теперь занимает должность. */}
              {vacancy.title && vacancy.company && (
                <p className="mt-0.5 text-sm text-muted">{vacancy.company}</p>
              )}
              <a
                href={vacancy.url}
                target="_blank"
                rel="noreferrer"
                className="mt-1 inline-flex items-center gap-1.5 text-sm text-muted underline-offset-2 hover:text-foreground hover:underline"
              >
                <span className="truncate">{vacancy.url}</span>
                <ExternalLink className="size-3.5 shrink-0" aria-hidden="true" />
              </a>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <ActivityBadge vacancy={vacancy} />
              <Button size="sm" onClick={() => setEditOpen(true)}>
                <Pencil className="size-3.5" aria-hidden="true" />
                Изменить
              </Button>
            </div>
          </header>

          <VacancyFormDialog open={editOpen} onClose={() => setEditOpen(false)} vacancy={vacancy} />

          <section className="mt-6 rounded-lg border border-border bg-surface p-4">
            <h2 className="text-sm font-medium text-muted">Вакансия</h2>
            <dl className="mt-3 grid gap-3 sm:grid-cols-2">
              <Field label="Грейд">
                {vacancy.grade ? (GRADE_LABELS[vacancy.grade as Grade] ?? vacancy.grade) : '—'}
              </Field>
              <Field label="Формат работы">{formatWorkFormat(vacancy.work_format)}</Field>
              <Field label="Вилка из объявления">
                <span>{formatSalary(vacancy)}</span>
                {formatSalaryGross(vacancy.salary_gross) && (
                  <span className="ml-1.5 text-xs text-muted">
                    {formatSalaryGross(vacancy.salary_gross)}
                  </span>
                )}
              </Field>
              <Field label="Открыта">{formatDate(vacancy.opened_date)}</Field>
              <Field label="Добавлена">{formatDateTime(vacancy.created_at)}</Field>
              <Field label="Изменена">{formatDateTime(vacancy.updated_at)}</Field>
              <Field label="Технологии">
                {vacancy.tech_tags.length > 0 ? (
                  <ul className="flex flex-wrap gap-1">
                    {vacancy.tech_tags.map((tag) => (
                      <li key={tag}>
                        <Badge variant="outline">{tag}</Badge>
                      </li>
                    ))}
                  </ul>
                ) : (
                  '—'
                )}
              </Field>
            </dl>
          </section>

          <ActivityPanel vacancy={vacancy} />

          <CandidateStatusForm vacancyId={vacancy.id} status={vacancy.candidate_status} />

          <StagePlaceholder
            title="Резюме собеседования"
            stage="Этап 3"
            description="Оценка нейронки, прокторинг, оценки проверяющего и кандидата. Данные формируются внешними системами — трекер будет их хранить и показывать."
          />

          <StagePlaceholder
            title="Нейроблок"
            stage="Этап 3"
            description="Прогноз зарплаты по вакансии и оценка соответствия рынку. Запускается кнопкой, с выбором LLM-провайдера."
          />
        </article>
      )}
    </div>
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

function NotFound() {
  return (
    <div className="mx-auto max-w-md p-10 text-center">
      <h1 className="text-lg font-semibold">Вакансия не найдена</h1>
      <Link
        to="/vacancies"
        className="mt-4 inline-block text-sm text-primary underline-offset-2 hover:underline"
      >
        Вернуться к списку
      </Link>
    </div>
  )
}
