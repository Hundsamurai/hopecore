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
    title: 'Go-разработчик',
    company: 'Альфа',
    grade: 'senior',
    tech_tags: ['go', 'sqlite'],
    opened_date: '2026-08-01',
    salary_from: 300000,
    salary_to: 450000,
    salary_currency: 'RUB',
    salary_gross: true,
    work_format: 'remote',
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
    // Пустая вакансия: ни должности, ни компании, ни вилки — только ссылка.
    title: '',
    company: '',
    grade: '',
    tech_tags: [],
    opened_date: null,
    salary_from: null,
    salary_to: null,
    salary_currency: '',
    salary_gross: null,
    work_format: '',
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
    '/src/api/llm.ts',
    '/src/components/llm/RunStatusBadge.tsx',
    '/src/components/llm/RunDetailDialog.tsx',
    '/src/pages/LlmRunsPage.tsx',
    '/src/api/backups.ts',
    '/src/pages/BackupsPage.tsx',
    '/src/components/vacancies/ExtractPreviewDialog.tsx',
    '/src/components/vacancies/ExtractButton.tsx',
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
      // Раздел журнала стал рабочим, а поиск по источникам отложен до Этапа 2.1
      'Запуски LLM',
      'Резервные копии',
      'Поиск вакансий',
      'Этап 2.1',
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
      'Вакансия',
      'Грейд',
      'Вилка',
      'Технологии',
      'Активность',
      'Отклик',
      'Этап',
      'Изменена',
      // Данные строк
      'Go-разработчик',
      'Альфа',
      'tbank.ru',
      'Senior',
      'sqlite',
      'техническое интервью',
      'активна',
      // Вилка с неразрывными пробелами из Intl и пояснением про налоги
      '450',
      'до вычета налогов',
      'Удалённо',
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
      'Должность',
      'Компания',
      'Грейд',
      'Дата открытия',
      'Технологии',
      // Новые поля Этапа 2
      'Зарплата из объявления',
      'Валюта',
      'До вычета налогов',
      'На руки',
      'Формат работы',
      'Удалённо',
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
    [
      'Изменить вакансию',
      'https://example.com/jobs/1',
      'Go-разработчик',
      'Альфа',
      '2026-08-01',
      'Убрать тег go',
      // Вилка и формат подставились в поля формы
      'value="300000"',
      'value="450000"',
      'value="RUB"',
    ],
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

  // Экран журнала: без данных должно быть понятное пустое состояние.
  const { LlmRunsPage } = await server.ssrLoadModule('/src/pages/LlmRunsPage.tsx')
  check(
    renderToString(
      withProviders(
        React.createElement(
          MemoryRouter,
          { initialEntries: ['/llm/runs'] },
          React.createElement(LlmRunsPage),
        ),
      ),
    ),
    ['Запуски LLM', 'инициируете вы'],
    'экран журнала',
  )

  // Экран резервных копий: пустое состояние объясняет, зачем раздел нужен.
  const { BackupsPage } = await server.ssrLoadModule('/src/pages/BackupsPage.tsx')
  const { backupKeys } = await server.ssrLoadModule('/src/api/backups.ts')

  /** Рендер страницы с заранее положенным в кэш ответом API. */
  const renderBackups = (data) => {
    const client = new QueryClient()
    if (data) {
      client.setQueryData(backupKeys.list, data)
    }
    return renderToString(
      React.createElement(
        QueryClientProvider,
        { client },
        React.createElement(
          ToastProvider,
          null,
          React.createElement(
            MemoryRouter,
            { initialEntries: ['/backups'] },
            React.createElement(BackupsPage),
          ),
        ),
      ),
    )
  }

  check(
    renderBackups({ items: [], dir: '/data/backups', total_bytes: 0 }),
    ['Резервные копии', 'Создать копию', 'Копий пока нет', '/data/backups'],
    'экран копий без данных',
  )

  const backupsHtml = renderBackups({
    items: [
      {
        name: 'hopecore-20260810-153000-before-restore.db',
        size_bytes: 143360,
        created_at: '2026-08-10T15:30:00Z',
        automatic: true,
      },
      {
        name: 'hopecore-20260810-120000.db',
        size_bytes: 139264,
        created_at: '2026-08-10T12:00:00Z',
        automatic: false,
      },
    ],
    dir: '/data/backups',
    total_bytes: 282624,
  })
  check(
    backupsHtml,
    [
      // Столбцы и данные
      'Копия',
      'Создана',
      'Размер',
      'hopecore-20260810-120000.db',
      '140 КБ',
      '276 КБ',
      '2 копии',
      // Автоматическую копию видно: именно она отменяет восстановление
      'снята автоматически перед восстановлением',
      // Действия
      'Восстановить',
      'Удалить копию hopecore-20260810-120000.db',
      // Предупреждение в диалоге подтверждения
      'Все текущие данные',
      'можно будет отменить',
      // Доступность
      '<caption',
      'scope="col"',
    ],
    'экран копий',
  )

  // Свежие копии сверху: восстанавливают обычно последнюю.
  if (
    backupsHtml.indexOf('hopecore-20260810-153000-before-restore.db') >
    backupsHtml.indexOf('hopecore-20260810-120000.db')
  ) {
    throw new Error('экран копий: ожидался порядок «свежие сверху»')
  }
  console.log('ok render экран копий — свежие сверху')

  // Статусы различимы не только цветом: подпись всегда рядом с бейджем.
  const { RunStatusBadge } = await server.ssrLoadModule('/src/components/llm/RunStatusBadge.tsx')
  const { RUN_STATUS_LABELS } = await server.ssrLoadModule('/src/api/llm.ts')
  for (const [status, label] of Object.entries(RUN_STATUS_LABELS)) {
    const html = renderToString(React.createElement(RunStatusBadge, { status }))
    if (!html.includes(label)) {
      throw new Error(`статус ${status}: в разметке нет подписи «${label}»`)
    }
  }
  console.log('ok render бейджи статусов —', Object.keys(RUN_STATUS_LABELS).join(', '))

  // Форматирование расхода: прочерк вместо выдуманной суммы.
  const { formatBytes, formatCost, formatTokens, formatDuration } =
    await server.ssrLoadModule('/src/lib/format.ts')
  const formatChecks = [
    [formatCost(null), '—', 'стоимость без прайса'],
    [formatCost(0.0032), '$0.0032', 'стоимость тысячными'],
    [formatCost(2.8), '$2.80', 'стоимость обычная'],
    [formatTokens(null), '—', 'токены не вернулись'],
    [formatDuration(830), '830 мс', 'меньше секунды'],
    [formatDuration(1843), '1.8 с', 'больше секунды'],
    [formatBytes(512), '512 Б', 'размер в байтах'],
    [formatBytes(143360), '140 КБ', 'размер в килобайтах'],
    [formatBytes(5 * 1024 * 1024), '5.0 МБ', 'размер в мегабайтах'],
  ]
  for (const [got, want, what] of formatChecks) {
    if (got !== want) {
      throw new Error(`${what}: получено «${got}», ожидалось «${want}»`)
    }
    console.log('ok формат —', what, '→', got)
  }

  // Предпросмотр извлечения на ответе, похожем на настоящий: у Сбера
  // на странице нет вилки, зато есть должность, компания и технологии.
  const { ExtractPreviewDialog } = await server.ssrLoadModule(
    '/src/components/vacancies/ExtractPreviewDialog.tsx',
  )

  const extractResult = {
    run_id: 5,
    provider: 'gemini',
    model: 'gemini-2.5-flash',
    source_url: 'https://rabota.sber.ru/search/middle-golang-razrabochik-4554633/',
    source_chars: 2921,
    warnings: [],
    fields: {
      title: { extracted: 'Middle Golang разрабочик', current: null, has_value: true, differs: true },
      company: { extracted: 'ПАО Сбербанк', current: null, has_value: true, differs: true },
      grade: { extracted: 'middle', current: 'junior', has_value: true, differs: true },
      tech_tags: {
        extracted: ['Go', 'PostgreSQL', 'Kubernetes'],
        current: null,
        has_value: true,
        differs: true,
      },
      opened_date: { extracted: '2026-08-05', current: null, has_value: true, differs: true },
      // Вилки на странице нет — модель вернула null, и это правильный ответ.
      salary_from: { extracted: null, current: null, has_value: false, differs: false },
      salary_to: { extracted: null, current: null, has_value: false, differs: false },
      salary_currency: { extracted: null, current: null, has_value: false, differs: false },
      salary_gross: { extracted: null, current: null, has_value: false, differs: false },
      // Совпадает с тем, что уже в карточке: галочка не нужна.
      work_format: { extracted: 'onsite', current: 'onsite', has_value: true, differs: false },
    },
    usage: {
      input_tokens: 1071,
      output_tokens: 106,
      cost_estimate: 0.002131,
      attempts: 1,
      duration_ms: 1843,
    },
  }

  const previewHtml = renderToString(
    withProviders(
      React.createElement(
        MemoryRouter,
        null,
        React.createElement(ExtractPreviewDialog, {
          vacancyID: 6,
          result: extractResult,
          onClose: () => {},
        }),
      ),
    ),
  )

  check(
    previewHtml,
    [
      'Что нашла модель',
      'gemini-2.5-flash',
      'Сейчас',
      'Модель предлагает',
      // Значения приведены к человеческому виду
      'Middle Golang разрабочик',
      'ПАО Сбербанк',
      'Middle',
      'Go, PostgreSQL, Kubernetes',
      '05.08.2026',
      'без изменений',
      // Пять полей отличаются, они и отмечены
      'Применить отмеченные (5)',
      // Расход и ссылка на журнал
      '$0.0021',
      '1.8 с',
      // React вставляет разделитель между текстом и выражением,
      // поэтому номер запуска в разметке идёт отдельным узлом.
      'запуск №',
      '/llm/runs',
      // Доступность
      'Применить поле «Должность»',
      '<caption',
      'scope="col"',
    ],
    'предпросмотр извлечения',
  )

  // Совпадающее поле нельзя отметить: применять там нечего.
  if (!previewHtml.includes('disabled')) {
    throw new Error('предпросмотр: у неизменившихся полей чекбокс должен быть заблокирован')
  }
  console.log('ok render предпросмотр — совпадающие поля заблокированы')

  // Пустой результат: модель нашла только то, что уже есть.
  const nothingNew = {
    ...extractResult,
    fields: Object.fromEntries(
      Object.entries(extractResult.fields).map(([name, field]) => [
        name,
        { ...field, differs: false },
      ]),
    ),
  }
  const nothingHtml = renderToString(
    withProviders(
      React.createElement(
        MemoryRouter,
        null,
        React.createElement(ExtractPreviewDialog, {
          vacancyID: 6,
          result: nothingNew,
          onClose: () => {},
        }),
      ),
    ),
  )
  check(
    nothingHtml,
    ['Модель не нашла ничего нового', 'Применить отмеченные (0)'],
    'предпросмотр без изменений',
  )

  console.log('\nсмоук пройден')
} finally {
  await server.close()
}
