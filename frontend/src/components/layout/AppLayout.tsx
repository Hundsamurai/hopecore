import { useCallback, useEffect, useState } from 'react'
import { Outlet } from 'react-router-dom'

import { Sidebar } from './Sidebar'

const COLLAPSED_KEY = 'hopecore.sidebar.collapsed'

/**
 * Состояние сайдбара живёт в localStorage: свернул один раз — осталось
 * свёрнутым после перезагрузки. Ради этого сервер трогать незачем.
 */
function readCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === 'true'
  } catch {
    // Приватный режим может запрещать localStorage — не повод падать.
    return false
  }
}

export function AppLayout() {
  const [collapsed, setCollapsed] = useState(readCollapsed)

  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_KEY, String(collapsed))
    } catch {
      // Ничего страшного: настройка просто не сохранится между сессиями.
    }
  }, [collapsed])

  const toggle = useCallback(() => setCollapsed((value) => !value), [])

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar collapsed={collapsed} onToggle={toggle} />
      <main className="flex-1 overflow-y-auto">
        <Outlet />
      </main>
    </div>
  )
}
