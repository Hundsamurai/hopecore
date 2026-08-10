import { X } from 'lucide-react'
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'

import { cn } from '@/lib/utils'

type ToastVariant = 'info' | 'success' | 'danger'

interface Toast {
  id: number
  variant: ToastVariant
  title: string
  description?: string
}

interface ToastContextValue {
  show: (toast: Omit<Toast, 'id'>) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

/** Через сколько уведомление уезжает само. Ошибки живут дольше: их читают. */
const LIFETIME: Record<ToastVariant, number> = {
  info: 5000,
  success: 5000,
  danger: 10000,
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const show = useCallback(
    (toast: Omit<Toast, 'id'>) => {
      const id = nextId.current++
      setToasts((current) => [...current, { ...toast, id }])
      window.setTimeout(() => dismiss(id), LIFETIME[toast.variant])
    },
    [dismiss],
  )

  const value = useMemo(() => ({ show }), [show])

  return (
    <ToastContext.Provider value={value}>
      {children}

      {/*
        role="status" + aria-live: сводка массовой проверки должна доходить
        и до тех, кто не смотрит в правый нижний угол.
      */}
      <div
        role="status"
        aria-live="polite"
        className="pointer-events-none fixed right-4 bottom-4 z-50 flex w-[min(24rem,calc(100vw-2rem))] flex-col gap-2"
      >
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={cn(
              'pointer-events-auto flex items-start gap-3 rounded-lg border bg-surface p-3 shadow-lg',
              toast.variant === 'success' && 'border-success/40',
              toast.variant === 'danger' && 'border-danger/40',
              toast.variant === 'info' && 'border-border',
            )}
          >
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{toast.title}</p>
              {toast.description && (
                <p className="mt-0.5 text-xs whitespace-pre-line text-muted">{toast.description}</p>
              )}
            </div>
            <button
              type="button"
              onClick={() => dismiss(toast.id)}
              aria-label="Закрыть уведомление"
              className="shrink-0 text-muted transition-colors hover:text-foreground"
            >
              <X className="size-3.5" aria-hidden="true" />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext)
  if (!context) {
    throw new Error('useToast использован вне ToastProvider')
  }
  return context
}
