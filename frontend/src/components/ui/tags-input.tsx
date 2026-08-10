import { X } from 'lucide-react'
import { useState, type KeyboardEvent } from 'react'

import { cn } from '@/lib/utils'

interface TagsInputProps {
  value: string[]
  onChange: (value: string[]) => void
  id?: string
  describedBy?: string
  invalid?: boolean
  placeholder?: string
}

/**
 * Ввод тегов технологий. Порядок сохраняется: он несёт смысл — сначала главные
 * технологии вакансии. Дубликаты не добавляются, регистр сохраняется как введён
 * (backend теги не нормализует, только обрезает пробелы).
 */
export function TagsInput({
  value,
  onChange,
  id,
  describedBy,
  invalid,
  placeholder = 'go, postgres — Enter или запятая',
}: TagsInputProps) {
  const [draft, setDraft] = useState('')

  const commit = (raw: string) => {
    const tag = raw.trim()
    if (!tag) {
      return
    }
    if (!value.some((existing) => existing.toLowerCase() === tag.toLowerCase())) {
      onChange([...value, tag])
    }
    setDraft('')
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' || event.key === ',') {
      // Enter внутри формы отправил бы её — тег важнее.
      event.preventDefault()
      commit(draft)
      return
    }
    // Backspace на пустом поле удаляет последний тег: привычное поведение чипсов.
    if (event.key === 'Backspace' && draft === '' && value.length > 0) {
      onChange(value.slice(0, -1))
    }
  }

  return (
    <div
      className={cn(
        'flex flex-wrap items-center gap-1.5 rounded-md border bg-background px-2 py-1.5',
        invalid ? 'border-danger' : 'border-border',
      )}
    >
      {value.map((tag) => (
        <span
          key={tag}
          className="inline-flex items-center gap-1 rounded-full bg-surface-hover px-2 py-0.5 text-xs"
        >
          {tag}
          <button
            type="button"
            onClick={() => onChange(value.filter((item) => item !== tag))}
            aria-label={`Убрать тег ${tag}`}
            className="text-muted transition-colors hover:text-danger"
          >
            <X className="size-3" aria-hidden="true" />
          </button>
        </span>
      ))}

      <input
        id={id}
        aria-describedby={describedBy}
        aria-invalid={invalid || undefined}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={handleKeyDown}
        // Незакоммиченный текст не должен теряться при отправке формы.
        onBlur={() => commit(draft)}
        placeholder={value.length === 0 ? placeholder : ''}
        className="min-w-32 flex-1 bg-transparent py-0.5 text-sm outline-none placeholder:text-muted/60"
      />
    </div>
  )
}
