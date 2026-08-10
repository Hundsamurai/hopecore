import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * cn склеивает классы и разрешает конфликты Tailwind: последний класс
 * той же группы побеждает. Нужен, чтобы пропс className мог переопределять
 * стили компонента без !important.
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
