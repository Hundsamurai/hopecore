import {
  createColumnHelper,
  createSortedRowModel,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_basic,
  sortFn_datetime,
  sortFn_text,
  tableFeatures,
  useTable,
  type SortFn,
} from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ArrowUpDown, ExternalLink } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'

import { GRADE_LABELS, type Grade, type Vacancy } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import {
  formatDate,
  formatDateTime,
  formatHost,
  formatSalary,
  formatSalaryGross,
  formatWorkFormat,
  salarySortValue,
  vacancyHeading,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import { ActivityBadge } from './ActivityBadge'

/**
 * Набор возможностей таблицы объявляется явно (API TanStack Table v9):
 * в бандл попадает только сортировка, всё остальное не подключается.
 */
const features = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: {
    alphanumeric: sortFn_alphanumeric,
    basic: sortFn_basic,
    datetime: sortFn_datetime,
    text: sortFn_text,
  },
})

const columnHelper = createColumnHelper<typeof features, Vacancy>()

/** Порядок грейдов: алфавитная сортировка для них бессмысленна. */
const GRADE_ORDER: Record<string, number> = {
  intern: 1,
  junior: 2,
  middle: 3,
  senior: 4,
  lead: 5,
}

const sortByGrade: SortFn<typeof features, Vacancy> = (rowA, rowB) =>
  (GRADE_ORDER[rowA.original.grade] ?? 0) - (GRADE_ORDER[rowB.original.grade] ?? 0)

/** Сначала снятые: при разборе списка интересны именно они. */
const sortByActivity: SortFn<typeof features, Vacancy> = (rowA, rowB) =>
  Number(rowA.original.is_active) - Number(rowB.original.is_active)

const columns = columnHelper.columns([
  columnHelper.accessor((row) => vacancyHeading(row), {
    id: 'position',
    header: 'Вакансия',
    sortFn: 'text',
    cell: ({ row, getValue }) => (
      <div className="flex min-w-0 flex-col">
        {/* Ссылка, а не обработчик на строке: карточка открывается с клавиатуры,
            работает Cmd-клик в новой вкладке и виден адрес перехода. */}
        <Link
          to={`/vacancies/${row.original.id}`}
          className="truncate font-medium underline-offset-2 hover:underline"
        >
          {getValue()}
        </Link>
        {/* Под должностью — компания, а если её нет, то домен: что-то опознаваемое. */}
        <span className="truncate text-xs text-muted">
          {row.original.title
            ? row.original.company || formatHost(row.original.url)
            : formatHost(row.original.url)}
        </span>
      </div>
    ),
  }),

  columnHelper.accessor('grade', {
    header: 'Грейд',
    sortFn: sortByGrade,
    cell: ({ row, getValue }) => {
      const grade = getValue()
      const format = row.original.work_format

      return (
        <div className="flex flex-col gap-0.5">
          {grade ? (
            <span className="text-sm">{GRADE_LABELS[grade as Grade] ?? grade}</span>
          ) : (
            <span className="text-muted">—</span>
          )}
          {format && (
            <span className="text-xs whitespace-nowrap text-muted">
              {formatWorkFormat(format)}
            </span>
          )}
        </div>
      )
    },
  }),

  columnHelper.accessor((row) => salarySortValue(row), {
    id: 'salary',
    header: 'Вилка',
    sortFn: 'basic',
    // Вакансии без указанной вилки уезжают в конец при любом направлении.
    sortUndefined: 'last',
    cell: ({ row }) => {
      const gross = formatSalaryGross(row.original.salary_gross)
      return (
        <div className="flex flex-col gap-0.5">
          <span className="text-sm whitespace-nowrap">{formatSalary(row.original)}</span>
          {gross && <span className="text-xs whitespace-nowrap text-muted">{gross}</span>}
        </div>
      )
    },
  }),

  columnHelper.accessor('tech_tags', {
    header: 'Технологии',
    enableSorting: false,
    cell: ({ getValue }) => {
      const tags = getValue()
      if (tags.length === 0) {
        return <span className="text-muted">—</span>
      }
      return (
        <ul className="flex flex-wrap gap-1">
          {tags.map((tag) => (
            <li key={tag}>
              <Badge variant="outline">{tag}</Badge>
            </li>
          ))}
        </ul>
      )
    },
  }),

  columnHelper.accessor('is_active', {
    id: 'activity',
    header: 'Активность',
    sortFn: sortByActivity,
    cell: ({ row }) => <ActivityBadge vacancy={row.original} />,
  }),

  columnHelper.accessor((row) => row.candidate_status?.sent_at ?? undefined, {
    id: 'sent_at',
    header: 'Отклик',
    sortFn: 'datetime',
    // Вакансии без отклика уезжают в конец при любом направлении сортировки.
    sortUndefined: 'last',
    cell: ({ getValue }) => <span className="text-sm">{formatDate(getValue() ?? null)}</span>,
  }),

  columnHelper.accessor((row) => row.candidate_status?.interview_stage || undefined, {
    id: 'interview_stage',
    header: 'Этап',
    sortFn: 'text',
    sortUndefined: 'last',
    cell: ({ getValue }) => {
      const stage = getValue()
      return stage ? <span className="text-sm">{stage}</span> : <span className="text-muted">—</span>
    },
  }),

  columnHelper.accessor('updated_at', {
    header: 'Изменена',
    sortFn: 'datetime',
    cell: ({ getValue }) => (
      <span className="text-sm whitespace-nowrap text-muted">{formatDateTime(getValue())}</span>
    ),
  }),

  columnHelper.display({
    id: 'link',
    header: '',
    cell: ({ row }) => (
      <a
        href={row.original.url}
        target="_blank"
        rel="noreferrer"
        onClick={(event) => event.stopPropagation()}
        className="inline-flex text-muted transition-colors hover:text-foreground"
        aria-label={`Открыть вакансию ${vacancyHeading(row.original)} на сайте`}
        title="Открыть на сайте"
      >
        <ExternalLink className="size-4" aria-hidden="true" />
      </a>
    ),
  }),
])

interface VacancyTableProps {
  vacancies: Vacancy[]
}

export function VacancyTable({ vacancies }: VacancyTableProps) {
  const navigate = useNavigate()

  const table = useTable({
    features,
    columns,
    data: vacancies,
    // Дефолт совпадает с порядком, который отдаёт сервер: свежеизменённые сверху.
    initialState: { sorting: [{ id: 'updated_at', desc: true }] },
    // Сортировка на клиенте: список приходит целиком (десятки-сотни записей),
    // зато сортировать можно по любому столбцу, включая этап и дату отклика,
    // которых нет в белом списке сортировки на сервере.
  })

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full border-collapse text-left">
        <caption className="sr-only">
          Список вакансий: должность и компания, грейд с форматом работы, зарплатная вилка,
          технологии, активность, дата отклика, этап собеседования и дата изменения
        </caption>

        <thead className="bg-surface">
          {table.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id}>
              {headerGroup.headers.map((header) => {
                const sortDirection = header.column.getIsSorted()
                const sortable = header.column.getCanSort()

                return (
                  <th
                    key={header.id}
                    scope="col"
                    aria-sort={
                      sortDirection === 'asc'
                        ? 'ascending'
                        : sortDirection === 'desc'
                          ? 'descending'
                          : undefined
                    }
                    className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase"
                  >
                    {sortable ? (
                      <button
                        type="button"
                        onClick={header.column.getToggleSortingHandler()}
                        className="inline-flex items-center gap-1 transition-colors hover:text-foreground"
                      >
                        <table.FlexRender header={header} />
                        <SortIcon direction={sortDirection} />
                      </button>
                    ) : (
                      <table.FlexRender header={header} />
                    )}
                  </th>
                )
              })}
            </tr>
          ))}
        </thead>

        <tbody>
          {table.getRowModel().rows.map((row) => (
            <tr
              key={row.id}
              // Клик по строке — удобство для мыши. Клавиатурный путь идёт
              // через ссылку в первой колонке, поэтому строка не в tab-порядке.
              onClick={() => navigate(`/vacancies/${row.original.id}`)}
              className={cn(
                'cursor-pointer border-b border-border/60 transition-colors last:border-0 hover:bg-surface',
                !row.original.is_active && 'opacity-60',
              )}
            >
              {row.getAllCells().map((cell) => (
                <td key={cell.id} className="max-w-64 px-3 py-2 align-top">
                  <table.FlexRender cell={cell} />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function SortIcon({ direction }: { direction: false | 'asc' | 'desc' }) {
  if (direction === 'asc') {
    return <ArrowUp className="size-3" aria-hidden="true" />
  }
  if (direction === 'desc') {
    return <ArrowDown className="size-3" aria-hidden="true" />
  }
  return <ArrowUpDown className="size-3 opacity-40" aria-hidden="true" />
}
