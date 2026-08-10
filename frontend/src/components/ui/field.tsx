import { useId, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface FieldProps {
  label: string
  /** Сообщение об ошибке: приходит либо от клиентской проверки, либо из API. */
  error?: string
  hint?: string
  required?: boolean
  /**
   * Идентификаторы прокидываются в input, чтобы подпись, подсказка и ошибка
   * были связаны с полем для скринридера.
   */
  children: (ids: { id: string; describedBy: string | undefined; invalid: boolean }) => ReactNode
  className?: string
}

export function Field({ label, error, hint, required, children, className }: FieldProps) {
  const id = useId()
  const hintId = `${id}-hint`
  const errorId = `${id}-error`

  const describedBy = [hint && hintId, error && errorId].filter(Boolean).join(' ') || undefined

  return (
    <div className={cn('flex flex-col gap-1', className)}>
      <label htmlFor={id} className="text-xs text-muted">
        {label}
        {required && (
          <span className="ml-0.5 text-danger" aria-hidden="true">
            *
          </span>
        )}
      </label>

      {children({ id, describedBy, invalid: Boolean(error) })}

      {hint && !error && (
        <p id={hintId} className="text-xs text-muted/70">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} className="text-xs text-danger">
          {error}
        </p>
      )}
    </div>
  )
}

const controlClasses =
  'w-full rounded-md border bg-background px-2.5 py-1.5 text-sm text-foreground placeholder:text-muted/60 disabled:opacity-50'

export function inputClasses(invalid: boolean): string {
  return cn(controlClasses, invalid ? 'border-danger' : 'border-border')
}
