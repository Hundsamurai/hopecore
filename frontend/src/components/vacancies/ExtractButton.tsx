import { Sparkles } from 'lucide-react'
import { useState } from 'react'

import { useExtractVacancy, useLLMProviders, type ExtractResult } from '@/api/llm'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'

import { ExtractPreviewDialog } from './ExtractPreviewDialog'

interface ExtractButtonProps {
  vacancyID: number
}

/**
 * Кнопка «Заполнить через LLM».
 *
 * Провайдер и модель не выбираются вручную: пока настроен один провайдер,
 * берётся он и его модель по умолчанию. Диалог выбора появится, когда
 * провайдеров станет больше одного.
 */
export function ExtractButton({ vacancyID }: ExtractButtonProps) {
  const { show } = useToast()
  const { data: providers, isPending: providersPending } = useLLMProviders()
  const extract = useExtractVacancy(vacancyID)

  const [result, setResult] = useState<ExtractResult | null>(null)

  const provider = providers?.[0]
  const unavailable = !providersPending && !provider

  const run = () => {
    if (!provider) {
      return
    }

    extract.mutate(
      { provider: provider.id, model: provider.default_model },
      {
        onSuccess: setResult,
        onError: (error) => {
          show({
            variant: 'danger',
            title: 'Заполнить не удалось',
            description: error.message,
          })
        },
      },
    )
  }

  return (
    <>
      <Button
        onClick={run}
        disabled={extract.isPending || unavailable || providersPending}
        title={
          unavailable
            ? 'Провайдеры не настроены: добавьте ключ в .env'
            : provider
              ? `${provider.label} · ${provider.default_model}`
              : undefined
        }
      >
        <Sparkles
          className={cn('size-3.5', extract.isPending && 'animate-pulse')}
          aria-hidden="true"
        />
        {extract.isPending ? 'Читаем страницу…' : 'Заполнить через LLM'}
      </Button>

      <ExtractPreviewDialog
        vacancyID={vacancyID}
        result={result}
        onClose={() => setResult(null)}
      />
    </>
  )
}
