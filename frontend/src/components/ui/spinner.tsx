import { Loader2 } from 'lucide-react'

import { cn } from '@/lib/utils'

interface SpinnerProps {
  className?: string
  /** Текст для скринридера: анимация сама по себе ничего не сообщает. */
  label?: string
}

export function Spinner({ className, label = 'Загрузка' }: SpinnerProps) {
  return (
    <span role="status" className="inline-flex items-center gap-2">
      <Loader2 className={cn('size-4 animate-spin text-muted', className)} aria-hidden="true" />
      <span className="sr-only">{label}</span>
    </span>
  )
}
