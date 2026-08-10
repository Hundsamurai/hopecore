import { Bot, Briefcase, PanelLeftClose, PanelLeftOpen, Search } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { NavLink } from 'react-router-dom'

import { cn } from '@/lib/utils'

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  /** Заглушки будущих этапов остаются видимыми, но недоступными. */
  disabled?: boolean
  stageHint?: string
}

const NAV_ITEMS: NavItem[] = [
  { to: '/vacancies', label: 'Вакансии', icon: Briefcase },
  { to: '/search', label: 'Поиск LLM', icon: Search, disabled: true, stageHint: 'Этап 2' },
  { to: '/ai', label: 'Нейроблок', icon: Bot, disabled: true, stageHint: 'Этап 3' },
]

interface SidebarProps {
  collapsed: boolean
  onToggle: () => void
}

export function Sidebar({ collapsed, onToggle }: SidebarProps) {
  return (
    <aside
      className={cn(
        'flex shrink-0 flex-col border-r border-border bg-surface transition-[width] duration-200',
        collapsed ? 'w-14' : 'w-56',
      )}
    >
      <div className={cn('flex h-14 items-center border-b border-border', collapsed ? 'justify-center' : 'gap-2 px-3')}>
        {!collapsed && (
          <span className="flex-1 truncate text-sm font-semibold tracking-tight">
            Hopecore
          </span>
        )}
        <button
          type="button"
          onClick={onToggle}
          className="inline-flex size-8 items-center justify-center rounded-md text-muted transition-colors hover:bg-surface-hover hover:text-foreground"
          aria-label={collapsed ? 'Развернуть меню' : 'Свернуть меню'}
          aria-expanded={!collapsed}
        >
          {collapsed ? (
            <PanelLeftOpen className="size-4" aria-hidden="true" />
          ) : (
            <PanelLeftClose className="size-4" aria-hidden="true" />
          )}
        </button>
      </div>

      <nav aria-label="Разделы" className="flex flex-col gap-1 p-2">
        {NAV_ITEMS.map((item) => (
          <SidebarItem key={item.to} item={item} collapsed={collapsed} />
        ))}
      </nav>

      {!collapsed && (
        <p className="mt-auto p-3 text-xs leading-relaxed text-muted">
          Локальный инструмент. Данные лежат в SQLite рядом с приложением.
        </p>
      )}
    </aside>
  )
}

function SidebarItem({ item, collapsed }: { item: NavItem; collapsed: boolean }) {
  const { icon: Icon, label, stageHint, disabled, to } = item

  // Заголовок нужен и в свёрнутом виде (иконка без подписи), и для заглушек:
  // пользователь должен понимать, почему пункт неактивен.
  const title = disabled ? `${label} — ${stageHint}` : label

  if (disabled) {
    return (
      <span
        title={title}
        aria-disabled="true"
        className={cn(
          'flex cursor-not-allowed items-center gap-3 rounded-md px-2 py-2 text-sm text-muted/60',
          collapsed && 'justify-center px-0',
        )}
      >
        <Icon className="size-4 shrink-0" aria-hidden="true" />
        {!collapsed && (
          <>
            <span className="flex-1 truncate">{label}</span>
            <span className="rounded bg-surface-hover px-1.5 py-0.5 text-[10px] whitespace-nowrap">
              {stageHint}
            </span>
          </>
        )}
      </span>
    )
  }

  return (
    <NavLink
      to={to}
      title={title}
      className={({ isActive }) =>
        cn(
          'flex items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors',
          collapsed && 'justify-center px-0',
          isActive
            ? 'bg-surface-hover font-medium text-foreground'
            : 'text-muted hover:bg-surface-hover hover:text-foreground',
        )
      }
    >
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      {!collapsed && <span className="truncate">{label}</span>}
    </NavLink>
  )
}
