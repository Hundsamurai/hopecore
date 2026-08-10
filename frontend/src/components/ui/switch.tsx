import { useId } from 'react'

import { cn } from '@/lib/utils'

interface SwitchProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  /** Пояснение под подписью: зачем этот переключатель нужен. */
  hint?: string
  className?: string
}

/**
 * Переключатель на role="switch": скринридер объявляет состояние,
 * пробел и Enter работают из коробки, потому что это обычная кнопка.
 */
export function Switch({ checked, onChange, label, hint, className }: SwitchProps) {
  const labelId = useId()

  return (
    <div className={cn('flex items-center gap-2', className)}>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-labelledby={labelId}
        onClick={() => onChange(!checked)}
        className={cn(
          'relative h-5 w-9 shrink-0 rounded-full border transition-colors',
          checked ? 'border-primary bg-primary' : 'border-border bg-surface-hover',
        )}
      >
        <span
          className={cn(
            'absolute top-0.5 size-3.5 rounded-full bg-foreground transition-[left]',
            checked ? 'left-[1.125rem]' : 'left-0.5',
          )}
          aria-hidden="true"
        />
      </button>

      <label id={labelId} className="cursor-default text-sm text-muted select-none">
        {label}
        {hint && <span className="ml-1 text-xs text-muted/70">{hint}</span>}
      </label>
    </div>
  )
}
