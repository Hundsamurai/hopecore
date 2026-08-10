import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="mx-auto max-w-md p-10 text-center">
      <h1 className="text-lg font-semibold">Страница не найдена</h1>
      <p className="mt-2 text-sm text-muted">
        Возможно, раздел появится на следующих этапах.
      </p>
      <Link
        to="/vacancies"
        className="mt-4 inline-block text-sm text-primary underline-offset-2 hover:underline"
      >
        Вернуться к вакансиям
      </Link>
    </div>
  )
}
