import { Check } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { ApiError } from '@/api/client'
import { useUpsertCandidateStatus, type CandidateStatusInput } from '@/api/candidateStatus'
import type { CandidateStatus } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, inputClasses } from '@/components/ui/field'

interface CandidateStatusFormProps {
  vacancyId: number
  status: CandidateStatus | null
}

/**
 * Форма хранит зарплаты строками: пустое поле и «0» — разные вещи, а число
 * в состоянии не даёт различить «не заполнено» и «ноль».
 */
interface FormState {
  cover_letter: string
  sent_at: string
  interview_stage: string
  hr_contact: string
  interview_record_url: string
  offer_received: boolean
  offered_salary: string
  real_salary: string
  market_salary_data: string
}

const EMPTY_FORM: FormState = {
  cover_letter: '',
  sent_at: '',
  interview_stage: '',
  hr_contact: '',
  interview_record_url: '',
  offer_received: false,
  offered_salary: '',
  real_salary: '',
  market_salary_data: '',
}

function toFormState(status: CandidateStatus | null): FormState {
  if (!status) {
    return EMPTY_FORM
  }
  return {
    cover_letter: status.cover_letter,
    sent_at: status.sent_at ?? '',
    interview_stage: status.interview_stage,
    hr_contact: status.hr_contact,
    interview_record_url: status.interview_record_url,
    offer_received: status.offer_received,
    offered_salary: status.offered_salary === null ? '' : String(status.offered_salary),
    real_salary: status.real_salary === null ? '' : String(status.real_salary),
    market_salary_data: status.market_salary_data,
  }
}

function parseSalary(raw: string): number | null {
  const trimmed = raw.trim()
  return trimmed === '' ? null : Number(trimmed)
}

function validate(form: FormState): Record<string, string> {
  const errors: Record<string, string> = {}

  for (const key of ['offered_salary', 'real_salary'] as const) {
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

  const record = form.interview_record_url.trim()
  if (record !== '') {
    try {
      const parsed = new URL(record)
      if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
        errors.interview_record_url = 'Ожидается ссылка со схемой http или https'
      }
    } catch {
      errors.interview_record_url = 'Не похоже на ссылку'
    }
  }

  return errors
}

export function CandidateStatusForm({ vacancyId, status }: CandidateStatusFormProps) {
  // Снимок сохранённого состояния держим в ref: он нужен для сравнения внутри
  // эффекта, а лишний ре-рендер при его обновлении не требуется.
  const savedSnapshot = useRef<FormState>(toFormState(status))
  const [form, setForm] = useState<FormState>(savedSnapshot.current)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [generalError, setGeneralError] = useState('')
  const [saved, setSaved] = useState(false)

  const upsert = useUpsertCandidateStatus(vacancyId)

  // Данные могли обновиться извне (например, кеш перезапросил карточку),
  // но затирать несохранённый ввод пользователя нельзя.
  useEffect(() => {
    const fresh = toFormState(status)
    setForm((current) => (isSame(current, savedSnapshot.current) ? fresh : current))
    savedSnapshot.current = fresh
  }, [status])

  const dirty = !isSame(form, savedSnapshot.current)

  const patch = (changes: Partial<FormState>) => {
    setForm((prev) => ({ ...prev, ...changes }))
    setSaved(false)
  }

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault()

    const clientErrors = validate(form)
    if (Object.keys(clientErrors).length > 0) {
      setErrors(clientErrors)
      return
    }

    setErrors({})
    setGeneralError('')

    const payload: CandidateStatusInput = {
      cover_letter: form.cover_letter,
      sent_at: form.sent_at || null,
      interview_stage: form.interview_stage,
      hr_contact: form.hr_contact,
      interview_record_url: form.interview_record_url.trim(),
      offer_received: form.offer_received,
      offered_salary: parseSalary(form.offered_salary),
      real_salary: parseSalary(form.real_salary),
      market_salary_data: form.market_salary_data,
    }

    upsert.mutate(payload, {
      onSuccess: (fresh) => {
        // Ответ — это уже нормализованные сервером данные (обрезанные пробелы),
        // поэтому форму синхронизируем с ним, а не с локальным payload.
        const next = toFormState(fresh)
        savedSnapshot.current = next
        setForm(next)
        setSaved(true)
      },
      onError: (error) => {
        if (error instanceof ApiError && Object.keys(error.fields).length > 0) {
          setErrors(error.fields)
          return
        }
        setGeneralError(error instanceof Error ? error.message : String(error))
      },
    })
  }

  return (
    <section className="mt-4 rounded-lg border border-border bg-surface p-4">
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="text-sm font-medium">Статус кандидата</h2>
        {!status && <span className="text-xs text-muted">ещё не заполнен</span>}
      </div>

      <form onSubmit={handleSubmit} className="mt-4 flex flex-col gap-3" noValidate>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Дата отклика" error={errors.sent_at}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                type="date"
                value={form.sent_at}
                onChange={(event) => patch({ sent_at: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field label="Этап собеседования" error={errors.interview_stage}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.interview_stage}
                onChange={(event) => patch({ interview_stage: event.target.value })}
                placeholder="скрининг, техническое интервью…"
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field label="Контакт HR" error={errors.hr_contact} hint="Телеграм или телефон">
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                value={form.hr_contact}
                onChange={(event) => patch({ hr_contact: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field label="Ссылка на запись собеседования" error={errors.interview_record_url}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                type="url"
                inputMode="url"
                value={form.interview_record_url}
                onChange={(event) => patch({ interview_record_url: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field label="Предлагаемая ЗП" error={errors.offered_salary}>
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                type="number"
                min="0"
                step="1000"
                value={form.offered_salary}
                onChange={(event) => patch({ offered_salary: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>

          <Field
            label="Реальная ЗП"
            error={errors.real_salary}
            hint="Если известна из открытых источников"
          >
            {({ id, describedBy, invalid }) => (
              <input
                id={id}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                type="number"
                min="0"
                step="1000"
                value={form.real_salary}
                onChange={(event) => patch({ real_salary: event.target.value })}
                className={inputClasses(invalid)}
              />
            )}
          </Field>
        </div>

        <Field label="Данные о ЗП по рынку" error={errors.market_salary_data}>
          {({ id, describedBy, invalid }) => (
            <input
              id={id}
              aria-describedby={describedBy}
              aria-invalid={invalid || undefined}
              value={form.market_salary_data}
              onChange={(event) => patch({ market_salary_data: event.target.value })}
              placeholder="медиана по грейду, вилка в компании…"
              className={inputClasses(invalid)}
            />
          )}
        </Field>

        <Field label="Сопроводительное письмо" error={errors.cover_letter}>
          {({ id, describedBy, invalid }) => (
            <textarea
              id={id}
              aria-describedby={describedBy}
              aria-invalid={invalid || undefined}
              rows={5}
              value={form.cover_letter}
              onChange={(event) => patch({ cover_letter: event.target.value })}
              className={`${inputClasses(invalid)} resize-y`}
            />
          )}
        </Field>

        <Checkbox
          checked={form.offer_received}
          onChange={(offer_received) => patch({ offer_received })}
          label="Оффер получен"
        />

        {generalError && (
          <p role="alert" className="rounded-md bg-danger-surface px-2.5 py-2 text-xs text-danger">
            {generalError}
          </p>
        )}

        <div className="flex items-center gap-3">
          <Button variant="primary" type="submit" disabled={upsert.isPending || !dirty}>
            {upsert.isPending ? 'Сохраняем…' : 'Сохранить'}
          </Button>

          {/* aria-live, чтобы результат сохранения был слышен, а не только виден. */}
          <span aria-live="polite" className="text-xs text-muted">
            {saved && !dirty && (
              <span className="inline-flex items-center gap-1 text-success">
                <Check className="size-3.5" aria-hidden="true" />
                Сохранено
              </span>
            )}
            {dirty && 'Есть несохранённые изменения'}
          </span>
        </div>
      </form>
    </section>
  )
}

function isSame(a: FormState, b: FormState): boolean {
  return (Object.keys(a) as Array<keyof FormState>).every((key) => a[key] === b[key])
}
