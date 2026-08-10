/** Грейды вакансии — тот же набор, что в model.Grades на backend. */
export const GRADES = ['intern', 'junior', 'middle', 'senior', 'lead'] as const

export type Grade = (typeof GRADES)[number]

export const GRADE_LABELS: Record<Grade, string> = {
  intern: 'Стажёр',
  junior: 'Junior',
  middle: 'Middle',
  senior: 'Senior',
  lead: 'Lead',
}

/** Дата в формате YYYY-MM-DD (тип model.Date на backend). */
export type IsoDate = string

export interface CandidateStatus {
  id: number
  vacancy_id: number
  cover_letter: string
  sent_at: IsoDate | null
  interview_stage: string
  hr_contact: string
  interview_record_url: string
  offer_received: boolean
  offered_salary: number | null
  real_salary: number | null
  market_salary_data: string
  created_at: string
  updated_at: string
}

export interface Vacancy {
  id: number
  url: string
  company: string
  grade: string
  tech_tags: string[]
  opened_date: IsoDate | null

  /**
   * Отображаемая активность: manual ?? auto ?? true.
   * Исходные значения приходят рядом, чтобы UI мог показать, откуда взялось
   * решение, и предложить сбросить ручной override.
   */
  is_active: boolean
  /** Ручное решение расходится с результатом последней проверки. */
  activity_conflict: boolean
  auto_is_active: boolean | null
  manual_is_active: boolean | null

  last_checked_at: string | null
  last_check_code: number | null
  last_check_error: string

  created_at: string
  updated_at: string

  candidate_status: CandidateStatus | null
}

export interface VacancyList {
  items: Vacancy[]
}

/** Сводка массовой проверки активности (service.CheckSummary). */
export interface CheckSummary {
  checked: number
  skipped: number
  became_inactive: number
  unknown: number
  failed: number
}

export type SortField = 'updated_at' | 'created_at' | 'company' | 'opened_date' | 'grade'

export interface VacancyListParams {
  includeInactive?: boolean
  sort?: SortField
  order?: 'asc' | 'desc'
}
