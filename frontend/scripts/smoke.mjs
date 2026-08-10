// Ручной смоук фронта: грузит модули через SSR-конвейер Vite и рендерит
// оболочку и таблицу в строку. Ловит битые импорты и падения на первом рендере,
// которых tsc не видит (например, смену API TanStack Table).
//
// Запуск: npm run smoke
import { createServer } from 'vite'
import React from 'react'
import { renderToString } from 'react-dom/server'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider, QueryClient } from '@tanstack/react-query'

const server = await createServer({ server: { middlewareMode: true }, logLevel: 'error' })

/** Две вакансии с разными состояниями активности и заполненным статусом. */
const fixtures = [
  {
    id: 1,
    url: 'https://example.com/jobs/1',
    company: 'Альфа',
    grade: 'senior',
    tech_tags: ['go', 'sqlite'],
    opened_date: '2026-08-01',
    is_active: true,
    activity_conflict: false,
    auto_is_active: true,
    manual_is_active: null,
    last_checked_at: '2026-08-10T12:00:00Z',
    last_check_code: 200,
    last_check_error: '',
    created_at: '2026-08-01T10:00:00Z',
    updated_at: '2026-08-10T12:00:00Z',
    candidate_status: {
      id: 1,
      vacancy_id: 1,
      cover_letter: '',
      sent_at: '2026-08-05',
      interview_stage: 'техническое интервью',
      hr_contact: '@hr',
      interview_record_url: '',
      offer_received: false,
      offered_salary: 350000,
      real_salary: null,
      market_salary_data: '',
      created_at: '2026-08-05T10:00:00Z',
      updated_at: '2026-08-05T10:00:00Z',
    },
  },
  {
    id: 2,
    url: 'https://tbank.ru/career/42',
    company: '',
    grade: '',
    tech_tags: [],
    opened_date: null,
    // Сайт отдал 404, но пользователь считает вакансию живой:
    // проверяем и бейдж «активна», и признак расхождения.
    is_active: true,
    activity_conflict: true,
    auto_is_active: false,
    manual_is_active: true,
    last_checked_at: '2026-08-09T09:30:00Z',
    last_check_code: 404,
    last_check_error: '',
    created_at: '2026-08-02T10:00:00Z',
    updated_at: '2026-08-09T09:30:00Z',
    candidate_status: null,
  },
]

function check(html, expectations, scope) {
  for (const text of expectations) {
    if (!html.includes(text)) {
      throw new Error(`${scope}: в разметке нет ожидаемого текста «${text}»`)
    }
    console.log('ok render', scope, '—', text)
  }
}

try {
  const modules = [
    '/src/lib/utils.ts',
    '/src/lib/format.ts',
    '/src/lib/queryClient.ts',
    '/src/api/client.ts',
    '/src/api/types.ts',
    '/src/api/vacancies.ts',
    '/src/components/ui/button.tsx',
    '/src/components/ui/badge.tsx',
    '/src/components/ui/spinner.tsx',
    '/src/components/ui/switch.tsx',
    '/src/components/layout/Sidebar.tsx',
    '/src/components/layout/AppLayout.tsx',
    '/src/components/ui/dialog.tsx',
    '/src/components/ui/field.tsx',
    '/src/components/ui/tags-input.tsx',
    '/src/components/vacancies/ActivityBadge.tsx',
    '/src/components/vacancies/VacancyTable.tsx',
    '/src/components/vacancies/VacancyFormDialog.tsx',
    '/src/components/ui/checkbox.tsx',
    '/src/api/candidateStatus.ts',
    '/src/components/vacancies/CandidateStatusForm.tsx',
    '/src/components/vacancies/StagePlaceholder.tsx',
    '/src/components/ui/toast.tsx',
    '/src/api/activity.ts',
    '/src/components/vacancies/ActivityPanel.tsx',
    '/src/pages/VacanciesPage.tsx',
    '/src/pages/VacancyPage.tsx',
    '/src/pages/NotFoundPage.tsx',
    '/src/App.tsx',
  ]

  for (const id of modules) {
    await server.ssrLoadModule(id)
    console.log('ok import', id)
  }

  const App = (await server.ssrLoadModule('/src/App.tsx')).default
  const { VacancyTable } = await server.ssrLoadModule('/src/components/vacancies/VacancyTable.tsx')
  const { ToastProvider } = await server.ssrLoadModule('/src/components/ui/toast.tsx')

  /** Провайдеры в том же порядке, что в main.tsx. */
  const withProviders = (element) =>
    React.createElement(
      QueryClientProvider,
      { client: new QueryClient() },
      React.createElement(ToastProvider, null, element),
    )

  const shellHtml = renderToString(
    withProviders(
      React.createElement(
        MemoryRouter,
        { initialEntries: ['/vacancies'] },
        React.createElement(App),
      ),
    ),
  )
  check(
    shellHtml,
    [
      'Вакансии',
      'Поиск LLM',
      'Этап 2',
      'Нейроблок',
      'Этап 3',
      'Свернуть меню',
      'Показать неактивные',
      'Опросить все',
      'Добавить',
    ],
    'оболочка',
  )

  const tableHtml = renderToString(
    React.createElement(
      MemoryRouter,
      null,
      React.createElement(VacancyTable, { vacancies: fixtures }),
    ),
  )
  check(
    tableHtml,
    [
      // Заголовки столбцов
      'Компания',
      'Грейд',
      'Технологии',
      'Активность',
      'Отклик',
      'Этап',
      'Изменена',
      // Данные строк
      'Альфа',
      'tbank.ru',
      'Senior',
      'sqlite',
      'техническое интервью',
      'активна',
      // Признаки происхождения активности
      'задано вручную',
      'сайт сообщает обратное',
      // Доступность
      'aria-sort="descending"',
      '<caption',
      'scope="col"',
      '/vacancies/1',
    ],
    'таблица',
  )

  const rowsOrder = [tableHtml.indexOf('Альфа'), tableHtml.indexOf('tbank.ru')]
  if (rowsOrder[0] > rowsOrder[1]) {
    throw new Error('порядок строк неверный: ожидалась сортировка по дате изменения, новые сверху')
  }
  console.log('ok render таблица — порядок по updated_at desc')

  const { VacancyFormDialog } = await server.ssrLoadModule(
    '/src/components/vacancies/VacancyFormDialog.tsx',
  )

  const renderDialog = (vacancy) =>
    renderToString(
      withProviders(
        React.createElement(
          MemoryRouter,
          null,
          React.createElement(VacancyFormDialog, { open: true, onClose: () => {}, vacancy }),
        ),
      ),
    )

  check(
    renderDialog(undefined),
    [
      'Добавить вакансию',
      'Ссылка на вакансию',
      'Компания',
      'Грейд',
      'Дата открытия',
      'Технологии',
      'Сохранить',
      'Отмена',
      'type="date"',
      'Не указан',
      'Senior',
      'aria-label="Закрыть"',
    ],
    'диалог создания',
  )

  const editHtml = renderDialog(fixtures[0])
  check(
    editHtml,
    ['Изменить вакансию', 'https://example.com/jobs/1', 'Альфа', '2026-08-01', 'Убрать тег go'],
    'диалог правки',
  )

  const { CandidateStatusForm } = await server.ssrLoadModule(
    '/src/components/vacancies/CandidateStatusForm.tsx',
  )
  const { StagePlaceholder } = await server.ssrLoadModule(
    '/src/components/vacancies/StagePlaceholder.tsx',
  )

  const renderStatusForm = (status) =>
    renderToString(withProviders(React.createElement(CandidateStatusForm, { vacancyId: 1, status })))

  // Заполненный статус: значения должны подставиться в поля.
  check(
    renderStatusForm(fixtures[0].candidate_status),
    [
      'Статус кандидата',
      'Дата отклика',
      'Этап собеседования',
      'Контакт HR',
      'Ссылка на запись собеседования',
      'Предлагаемая ЗП',
      'Реальная ЗП',
      'Данные о ЗП по рынку',
      'Сопроводительное письмо',
      'Оффер получен',
      'Сохранить',
      '2026-08-05',
      'техническое интервью',
      '@hr',
      '350000',
    ],
    'форма статуса',
  )

  // Пустой статус: форма доступна, но помечена как незаполненная,
  // а кнопка сохранения заблокирована до первого изменения.
  const emptyStatusHtml = renderStatusForm(null)
  check(emptyStatusHtml, ['ещё не заполнен', 'disabled'], 'форма статуса без данных')

  const placeholderHtml = renderToString(
    React.createElement(StagePlaceholder, {
      title: 'Резюме собеседования',
      stage: 'Этап 3',
      description: 'Данные формируются внешними системами.',
    }),
  )
  check(placeholderHtml, ['Резюме собеседования', 'Этап 3'], 'заглушка этапа')

  const { ActivityPanel } = await server.ssrLoadModule(
    '/src/components/vacancies/ActivityPanel.tsx',
  )

  // Вакансия с ручным override «активна» при результате проверки 404:
  // переключатель должен стоять на «Активна», а код ответа быть виден.
  check(
    renderToString(withProviders(React.createElement(ActivityPanel, { vacancy: fixtures[1] }))),
    [
      'Активность',
      'Опросить сейчас',
      'Последняя проверка',
      'Код ответа',
      '404',
      'По проверке',
      'Активна',
      'Снята',
      'role="radiogroup"',
      'Ваше решение: активна',
    ],
    'панель активности',
  )

  const { describeSummary } = await server.ssrLoadModule('/src/api/activity.ts')
  const summaryText = describeSummary({
    checked: 4,
    skipped: 1,
    became_inactive: 2,
    unknown: 1,
    failed: 1,
  })
  for (const fragment of [
    'опрошено: 4',
    'снято с публикации: 2',
    'ответ неинформативен: 1',
    'без ответа: 1',
    'пропущено как закрытые вручную: 1',
  ]) {
    if (!summaryText.includes(fragment)) {
      throw new Error(`сводка проверки без фрагмента «${fragment}»: ${summaryText}`)
    }
  }
  console.log('ok сводка проверки —', summaryText)

  // Нулевые счётчики не должны попадать в текст: сводка остаётся короткой.
  const quietSummary = describeSummary({
    checked: 3,
    skipped: 0,
    became_inactive: 0,
    unknown: 0,
    failed: 0,
  })
  if (quietSummary !== 'опрошено: 3') {
    throw new Error(`ожидалась короткая сводка «опрошено: 3», получено: ${quietSummary}`)
  }
  console.log('ok сводка проверки — нулевые счётчики скрыты')

  console.log('\nсмоук пройден')
} finally {
  await server.close()
}
