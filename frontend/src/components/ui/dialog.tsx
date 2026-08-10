import { X } from 'lucide-react'
import { useEffect, useRef, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
  footer?: ReactNode
  className?: string
}

/**
 * Модальное окно на нативном <dialog>.
 *
 * Платформа сама даёт то, за чем обычно тянут Radix: роль dialog с aria-modal,
 * захват фокуса, закрытие по Escape и затемнение через ::backdrop. Для трёх
 * диалогов Этапа 1 этого достаточно, и зависимость не нужна.
 */
export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  className,
}: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = ref.current
    if (!dialog) {
      return
    }

    if (open && !dialog.open) {
      dialog.showModal()
    } else if (!open && dialog.open) {
      dialog.close()
    }
  }, [open])

  return (
    <dialog
      ref={ref}
      // Закрытие по Escape и по клику на ::backdrop приходит событием close.
      onClose={onClose}
      onCancel={(event) => {
        event.preventDefault()
        onClose()
      }}
      onClick={(event) => {
        // Клик именно по подложке, а не по содержимому: сам <dialog> занимает
        // всю область, поэтому сравниваем цель события с ним.
        if (event.target === ref.current) {
          onClose()
        }
      }}
      className={cn(
        'm-auto w-[min(32rem,calc(100vw-2rem))] rounded-lg border border-border bg-surface p-0 text-foreground backdrop:bg-black/60',
        className,
      )}
    >
      <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">{title}</h2>
          {description && <p className="mt-0.5 text-xs text-muted">{description}</p>}
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Закрыть"
          className="inline-flex size-7 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-surface-hover hover:text-foreground"
        >
          <X className="size-4" aria-hidden="true" />
        </button>
      </div>

      <div className="px-4 py-4">{children}</div>

      {footer && (
        <div className="flex justify-end gap-2 border-t border-border px-4 py-3">{footer}</div>
      )}
    </dialog>
  )
}
