import { useEffect, useState } from 'react'

import { ApiError } from '@/api/client'
import {
  DEFAULT_CURRENCY,
  GRADES,
  GRADE_LABELS,
  WORK_FORMATS,
  WORK_FORMAT_LABELS,
  type Vacancy,
} from '@/api/types'
import { useCreateVacancy, useUpdateVacancy, type VacancyInput } from '@/api/vacancies'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Field, inputClasses } from '@/components/ui/field'
import { TagsInput } from '@/components/ui/tags-input'

interface VacancyFormDialogProps {
  open: boolean
  onClose: () => void
  /** Без вакансии диалог работает как «добавить», с вакансией — как «изменить». */
  vacancy?: Vacancy
}

/**
 * Зарплаты в состоянии формы — строки: пустое поле и «0» это разные вещи,
 * а число их не различает. Так же сделано в форме статуса кандидата.
 */
interface FormState {
  url: string
  title: string
  company: string
  grade: string
  tech_tags: string[]
  opened_date: string | null
  salary_from: string
  salary_to: string
  salary_currency: string
  /** Три состояния признака «до вычета налогов»: '', 'gross', 'net'. */
  salary_gross: '' | 'gross' | 'net'
  work_format: string
}

const EMPTY_FORM: FormState = {
  url: '',
  title: '',
  company: '',
  grade: '',
  tech_tags: [],
  opened_date: null,
  salary_from: '',
  salary_to: '',
  salary_currency: '',
  salary_gross: '',
  work_format: '',
}

function toFormState(vacancy?: Vacancy): FormState {
  if (!vacancy) {
    return EMPTY_FORM
  }
  return {
    url: vacancy.url,
    title: vacancy.title,
    company: vacancy.company,
    grade: vacancy.grade,
    tech_tags: vacancy.tech_tags,
    opened_date: vacancy.opened_date,
    salary_from: vacancy.salary_from === null ? '' : String(vacancy.salary_from),
    salary_to: vacancy.salary_to === null ? '' : String(vacancy.salary_to),
    salary_currency: vacancy.salary_currency,
    salary_gross: vacancy.salary_gross === null ? '' : vacancy.salary_gross ? 'gross' : 'net',
    work_format: vacancy.work_format,
  }
}

function parseSalary(raw: string): number | null {
  const trimmed = raw.trim()
  return trimmed === '' ? null : Number(trimmed)
}

/** Клиентская проверка повторяет серверную, чтобы не гонять заведомо битый запрос. */
function validate(form: FormState): Record<string, string> {
  const errors: Record<string, string> = {}

  const url = form.url.trim()
  if (!url) {
    errors.url = 'Обязательное поле'
  } else {
    try {
      const parsed = new URL(url)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        errors.url = 'Ожидается ссылка со схемой http или https'
      }
    } catch {
      errors.url = 'Не похоже на ссылку'
    }
  }

  for (const key of ['salary_from', 'salary_to'] as const) {
    const raw = form[key].trim()
    if (raw === '') {
      continue
    }
    const value = Number(raw)
    if (!Number.isFinite(value)) {
      errors[key] = 'Ожидается число'
    } else if (value < 0) {
      errors[key] = 'Зарплата не может быть отрицательной'
    }
  }

  const from = parseSalary(form.salary_from)
  const to = parseSalary(form.salary_to)
  if (from !== null && to !== null && from > to && !errors.salary_from && !errors.salary_to) {
    errors.salary_to = 'Верхняя граница не может быть меньше нижней'
  }

  const currency = form.salary_currency.trim()
  if (currency !== '' && !/^[A-Za-z]{3}$/.test(currency)) {
    errors.salary_currency = 'Три латинские буквы, например RUB'
  }

  return errors
}

export function VacancyFormDialog({ open, onClose, vacancy }: VacancyFormDialogProps) {
  const isEdit = Boolean(vacancy)

  const [form, setForm] = useState<FormState>(() => toFormState(vacancy))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [generalError, setGeneralError] = useState('')

  const create = useCreateVacancy()
  const update = useUpdateVacancy(vacancy?.id ?? 0)
  const mutation = isEdit ? update : create

  // Диалог не размонтируется между открытиями, поэтому состояние сбрасывается явно.
  useEffect(() => {
    if (open) {
      setForm(toFormState(vacancy))
      setErrors({})
      setGeneralError('')
    }
  }, [open, vacancy])

  const patch = (changes: Partial<FormState>) => setForm((prev) => ({ ...prev, ...changes }))

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()

    const clientErrors = validate(form)
    if (Object.keys(clientErrors).length > 0) {
      setErrors(clientErrors)
      return
    }

    setErrors({})
    setGeneralError('')

    const payload: VacancyInput = {
      url: form.url.trim(),
      title: form.title.trim(),
      company: form.company.trim(),
      grade: form.grade,
      tech_tags: form.tech_tags,
      // Пустая дата — это null, а не пустая строка: сервер ждёт YYYY-MM-DD или null.
      opened_date: form.opened_date || null,
      salary_from: parseSalary(form.salary_from),
      salary_to: parseSalary(form.salary_to),
      salary_currency: form.salary_currency.trim().toUpperCase(),
      // Сервер сам уберёт валюту и этот признак, если вилки нет.
      salary_gross: form.salary_gross === '' ? null : form.salary_gross === 'gross',
      work_format: form.work_format,
    }

    mutation.mutate(payload, {
      onSuccess: onClose,
      onError: (error) => {
        // Серверные ошибки раскладываем по полям: у формы есть куда их показать.
        if (error instanceof ApiError && Object.keys(error.fields).length > 0) {
          setErrors(error.fields)
          return
        }
        setGeneralError(error instanceof Error ? error.message : String(error))
      },
    })
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={isEdit ? 'Изменить вакансию' : 'Добавить вакансию'}
      description={
        isEdit
          ? undefined
          : 'Ссылка обязательна, остальное можно заполнить позже. Автоматически ничего не запрашивается.'
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={mutation.isPending}>
            Отмена
          </Button>
          <Button
            variant="primary"
            type="submit"
            form="vacancy-form"
            disabled={mutation.isPending}
          >
            {mutation.isPending ? 'Сохраняем…' : 'Сохранить'}
          </Button>
        </>
      }
    >
      <form id="vacancy-form" onSubmit={handleSubmit} className="flex flex-col gap-3" noValidate>
        <Field label="Ссылка на вакансию" error={errors.url} required>
          {({ id, describedBy, invalid }) => (
            <input
              id={id}
              aria-describedby={describedBy}
              aria-invalid={invalid || undefined}
              type="url"
              inputMode="url"
              autoComplete="off"
              value={form.url}
              onChange={(event) => patch({ url: event.target.value })}
              placeholder="https://hh.ru/vacancy/123"
              className={inputClasses(invalid)}
            />
          )}
        </Field>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Должность" error={errors.title}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.title}
                onChange={(event) => patch({ title: event.target.value })}
                placeholder="Go-разработчик"
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field label="Компания" error={errors.company}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.company}
                onChange={(event) => patch({ company: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Грейд" error={errors.grade}>
            {({ id, describedBy, invalid }) => (
              <select
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.grade}
                onChange={(event) => patch({ grade: event.target.value })}
                className={inputClasses(invalid)}
              >
                <option value="">Не указан</option>
                {GRADES.map((grade) => (
                  <option key={grade} value={grade}>
                    {GRADE_LABELS[grade]}
                  </option>
                ))}
              </select>
            )}
          </Field>

          <Field label="Дата открытия" error={errors.opened_date}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                // Нативный date-инпут отдаёт ровно YYYY-MM-DD — формат API.
                type="date"
                value={form.opened_date ?? ''}
                onChange={(event) => patch({ opened_date: event.target.value || null })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>
        </div>

        <fieldset className="rounded-md border border-border p-3">
          <legend className="px-1 text-xs text-muted">Зарплата из объявления</legend>

          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="От" error={errors.salary_from}>
              {({ id, describedBy, invalid }) => (
                <input
                  id={id}
                  aria-describedby={describedBy}
                  aria-invalid={invalid || undefined}
                  type="number"
                  min="0"
                  step="10000"
                  value={form.salary_from}
                  onChange={(event) => patch({ salary_from: event.target.value })}
                  className={inputClasses(invalid)}
                />
              )}
            </Field>

            <Field label="До" error={errors.salary_to}>
              {({ id, describedBy, invalid }) => (
                <input
                  id={id}
                  aria-describedby={describedBy}
                  aria-invalid={invalid || undefined}
                  type="number"
                  min="0"
                  step="10000"
                  value={form.salary_to}
                  onChange={(event) => patch({ salary_to: event.target.value })}
                  className={inputClasses(invalid)}
                />
              )}
            </Field>

            <Field label="Валюта" error={errors.salary_currency}>
              {({ id, describedBy, invalid }) => (
                <input
                  id={id}
                  aria-describedby={describedBy}
                  aria-invalid={invalid || undefined}
                  value={form.salary_currency}
                  onChange={(event) => patch({ salary_currency: event.target.value.toUpperCase() })}
                  maxLength={3}
                  placeholder={DEFAULT_CURRENCY}
                  className={inputClasses(invalid)}
                />
              )}
            </Field>
          </div>

          <Field label="Указана" error={errors.salary_gross} className="mt-3">
            {({ id, describedBy, invalid }) => (
              <select
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.salary_gross}
                onChange={(event) =>
                  patch({ salary_gross: event.target.value as FormState['salary_gross'] })
                }
                className={inputClasses(invalid)}
              >
                <option value="">Не указано</option>
                <option value="gross">До вычета налогов</option>
                <option value="net">На руки</option>
              </select>
            )}
          </Field>

          <p className="mt-2 text-xs text-muted/70">
            Это вилка из объявления. То, что предложили лично вам, заполняется в статусе
            кандидата.
          </p>
        </fieldset>

        <Field label="Формат работы" error={errors.work_format}>
          {({ id, describedBy, invalid }) => (
            <select
              id={id}
              aria-describedby={describedBy}
              aria-invalid={invalid || undefined}
              value={form.work_format}
              onChange={(event) => patch({ work_format: event.target.value })}
              className={inputClasses(invalid)}
            >
              <option value="">Не указан</option>
              {WORK_FORMATS.map((format) => (
                <option key={format} value={format}>
                  {WORK_FORMAT_LABELS[format]}
                </option>
              ))}
            </select>
          )}
        </Field>

        <Field
          label="Технологии"
          error={errors.tech_tags}
          hint="Порядок сохраняется: сначала главные технологии"
        >
          {({ id, describedBy, invalid }) => (
            <TagsInput
              id={id}
              describedBy={describedBy}
              invalid={invalid}
              value={form.tech_tags}
              onChange={(tech_tags) => patch({ tech_tags })}
            />
          )}
        </Field>

        {generalError && (
          <p role="alert" className="rounded-md bg-danger-surface px-2.5 py-2 text-xs text-danger">
            {generalError}
          </p>
        )}
      </form>
    </Dialog>
  )
}
