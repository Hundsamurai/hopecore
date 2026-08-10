import { useId } from 'react'

import { cn } from '@/lib/utils'

interface CheckboxProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  className?: string
}

/**
 * Обычный input[type=checkbox] со стилями: браузер сам даёт доступное состояние
 * и клавиатурное управление, накрывать его div-ами незачем.
 */
export function Checkbox({ checked, onChange, label, className }: CheckboxProps) {
  const id = useId()

  return (
    <div className={cn('flex items-center gap-2', className)}>
      <input
        id={id}
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="size-4 shrink-0 accent-primary"
      />
      <label htmlFor={id} className="text-sm select-none">
        {label}
      </label>
    </div>
  )
}
