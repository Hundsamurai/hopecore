import { Lock } from 'lucide-react'

import { Badge } from '@/components/ui/badge'

interface StagePlaceholderProps {
  title: string
  stage: string
  description: string
}

/**
 * Заглушка раздела, который появится на следующих этапах. Показываем, а не
 * скрываем: пользователь видит, что место для данных предусмотрено,
 * и понимает, почему оно пустое.
 */
export function StagePlaceholder({ title, stage, description }: StagePlaceholderProps) {
  return (
    <section className="mt-4 rounded-lg border border-dashed border-border p-4">
      <div className="flex items-center gap-2">
        <Lock className="size-3.5 text-muted" aria-hidden="true" />
        <h2 className="text-sm font-medium text-muted">{title}</h2>
        <Badge variant="outline">{stage}</Badge>
      </div>
      <p className="mt-2 text-sm text-muted/80">{description}</p>
    </section>
  )
}
