import { Navigate, Route, Routes } from 'react-router-dom'

import { AppLayout } from '@/components/layout/AppLayout'
import { BackupsPage } from '@/pages/BackupsPage'
import { LlmRunsPage } from '@/pages/LlmRunsPage'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { VacanciesPage } from '@/pages/VacanciesPage'
import { VacancyPage } from '@/pages/VacancyPage'

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        {/* Единственный рабочий раздел Этапа 1 — он же точка входа. */}
        <Route index element={<Navigate to="/vacancies" replace />} />
        <Route path="/vacancies" element={<VacanciesPage />} />
        <Route path="/vacancies/:id" element={<VacancyPage />} />
        <Route path="/llm/runs" element={<LlmRunsPage />} />
        <Route path="/backups" element={<BackupsPage />} />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  )
}
