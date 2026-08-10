import { useEffect, useState } from 'react'

import { ApiError } from '@/api/client'
import { GRADES, GRADE_LABELS, type Vacancy } from '@/api/types'
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

type FormState = VacancyInput

const EMPTY_FORM: FormState = {
  url: '',
  company: '',
  grade: '',
  tech_tags: [],
  opened_date: null,
}

function toFormState(vacancy?: Vacancy): FormState {
  if (!vacancy) {
    return EMPTY_FORM
  }
  return {
    url: vacancy.url,
    company: vacancy.company,
    grade: vacancy.grade,
    tech_tags: vacancy.tech_tags,
    opened_date: vacancy.opened_date,
  }
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
      company: form.company.trim(),
      grade: form.grade,
      tech_tags: form.tech_tags,
      // Пустая дата — это null, а не пустая строка: сервер ждёт YYYY-MM-DD или null.
      opened_date: form.opened_date || null,
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
