import { useMutation, useQueryClient } from '@tanstack/react-query'

import { apiRequest } from './client'
import type { CandidateStatus } from './types'
import { vacancyKeys } from './vacancies'

/**
 * Тело PUT /candidate-status. PUT заменяет представление целиком, поэтому
 * форма всегда присылает все поля — отсутствующее означает пустое значение.
 */
export interface CandidateStatusInput {
  cover_letter: string
  sent_at: string | null
  interview_stage: string
  hr_contact: string
  interview_record_url: string
  offer_received: boolean
  offered_salary: number | null
  real_salary: number | null
  market_salary_data: string
}

export function useUpsertCandidateStatus(vacancyId: number) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: CandidateStatusInput) =>
      apiRequest<CandidateStatus>(`/vacancies/${vacancyId}/candidate-status`, {
        method: 'PUT',
        body: input,
      }),
    onSuccess: () => {
      // Сохранение статуса поднимает updated_at вакансии, поэтому обновить нужно
      // и карточку, и список: порядок строк в таблице меняется.
      void queryClient.invalidateQueries({ queryKey: vacancyKeys.all })
    },
  })
}
