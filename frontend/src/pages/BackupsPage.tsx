import { AlertCircle, AlertTriangle, Database, RotateCcw, Trash2 } from 'lucide-react'
import { useState } from 'react'

import {
  useBackups,
  useCreateBackup,
  useDeleteBackup,
  useRestoreBackup,
  type Backup,
} from '@/api/backups'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { useToast } from '@/components/ui/toast'
import { formatBytes, formatDateTime } from '@/lib/format'

/**
 * Экран резервных копий базы.
 *
 * Смысл раздела — обратимость: если данные испорчены неудачной правкой,
 * состояние возвращается целиком. Поэтому восстановление требует подтверждения
 * и само создаёт копию текущего состояния, из которой можно откатиться назад.
 */
export function BackupsPage() {
  const { show } = useToast()
  const { data, isPending, isError, error } = useBackups()

  const create = useCreateBackup()
  const restore = useRestoreBackup()
  const remove = useDeleteBackup()

  const [restoreTarget, setRestoreTarget] = useState<Backup | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Backup | null>(null)

  const handleCreate = () => {
    create.mutate(undefined, {
      onSuccess: (backup) => {
        show({
          variant: 'success',
          title: 'Копия создана',
          description: `${backup.name} — ${formatBytes(backup.size_bytes)}`,
        })
      },
      onError: (err: Error) => {
        show({ variant: 'danger', title: 'Не удалось создать копию', description: err.message })
      },
    })
  }

  const handleRestore = () => {
    if (!restoreTarget) {
      return
    }
    const name = restoreTarget.name

    restore.mutate(name, {
      onSuccess: (result) => {
        setRestoreTarget(null)
        show({
          variant: 'success',
          title: 'База восстановлена',
          // Имя страховочной копии показываем сразу: это путь назад,
          // если восстановились не туда.
          description:
            `Данные из ${result.restored}. Состояние до восстановления сохранено ` +
            `как ${result.safety_backup} — из него можно вернуться обратно.`,
        })
      },
      onError: (err: Error) => {
        show({
          variant: 'danger',
          title: 'Восстановление не выполнено',
          description: `${err.message}. Данные не изменены.`,
        })
      },
    })
  }

  const handleDelete = () => {
    if (!deleteTarget) {
      return
    }

    remove.mutate(deleteTarget.name, {
      onSuccess: () => {
        setDeleteTarget(null)
        show({ variant: 'info', title: 'Копия удалена' })
      },
      onError: (err: Error) => {
        show({ variant: 'danger', title: 'Не удалось удалить копию', description: err.message })
      },
    })
  }

  return (
    <div className="mx-auto max-w-4xl p-6">
      <header className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Резервные копии</h1>
          <p className="mt-1 text-sm text-muted">
            Снимок всей базы одним файлом. Если данные испорчены, состояние возвращается целиком.
          </p>
        </div>
        <Button variant="primary" onClick={handleCreate} disabled={create.isPending}>
          <Database className="size-4" aria-hidden="true" />
          {create.isPending ? 'Создаём…' : 'Создать копию'}
        </Button>
      </header>

      {isPending && (
        <div className="rounded-lg border border-border bg-surface p-6">
          <Spinner label="Загружаем список копий" />
        </div>
      )}

      {isError && (
        <div className="flex items-start gap-3 rounded-lg border border-danger/40 bg-danger-surface p-4">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
          <div>
            <p className="text-sm font-medium">Не удалось загрузить список копий</p>
            <p className="mt-1 text-sm text-muted">{error.message}</p>
          </div>
        </div>
      )}

      {data?.items.length === 0 && (
        <div className="rounded-lg border border-dashed border-border p-10 text-center">
          <p className="text-sm font-medium">Копий пока нет</p>
          <p className="mx-auto mt-1 max-w-md text-sm text-muted">
            Создайте копию перед тем, как править данные большими порциями. Файлы лежат
            в <code>{data.dir}</code> и переживают пересборку контейнера.
          </p>
        </div>
      )}

      {data && data.items.length > 0 && (
        <>
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full border-collapse text-left">
              <caption className="sr-only">
                Резервные копии базы: имя файла, время создания, размер и действия
              </caption>
              <thead className="bg-surface">
                <tr>
                  {['Копия', 'Создана', 'Размер', ''].map((title, index) => (
                    <th
                      key={title || index}
                      scope="col"
                      className="border-b border-border px-3 py-2 text-xs font-medium tracking-wide text-muted uppercase"
                    >
                      {title}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {data.items.map((backup) => (
                  <tr
                    key={backup.name}
                    className="border-b border-border/60 last:border-0 hover:bg-surface"
                  >
                    <td className="px-3 py-2">
                      <span className="font-mono text-sm">{backup.name}</span>
                      {backup.automatic && (
                        <span className="mt-0.5 block text-xs text-muted">
                          снята автоматически перед восстановлением
                        </span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-sm whitespace-nowrap text-muted">
                      {formatDateTime(backup.created_at)}
                    </td>
                    <td className="px-3 py-2 text-sm whitespace-nowrap tabular-nums">
                      {formatBytes(backup.size_bytes)}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          onClick={() => setRestoreTarget(backup)}
                          disabled={restore.isPending}
                        >
                          <RotateCcw className="size-3.5" aria-hidden="true" />
                          Восстановить
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          onClick={() => setDeleteTarget(backup)}
                          aria-label={`Удалить копию ${backup.name}`}
                          title="Удалить копию"
                        >
                          <Trash2 className="size-3.5" aria-hidden="true" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <p className="mt-2 text-xs text-muted">
            {`${data.items.length} ${pluralBackups(data.items.length)} занимают ` +
              `${formatBytes(data.total_bytes)}. Файлы лежат в `}
            <code>{data.dir}</code>
          </p>
        </>
      )}

      <Dialog
        open={restoreTarget !== null}
        onClose={() => setRestoreTarget(null)}
        title="Восстановить базу из копии?"
        description={restoreTarget?.name}
        footer={
          <>
            <Button variant="ghost" onClick={() => setRestoreTarget(null)}>
              Отмена
            </Button>
            <Button variant="danger" onClick={handleRestore} disabled={restore.isPending}>
              {restore.isPending ? 'Восстанавливаем…' : 'Восстановить'}
            </Button>
          </>
        }
      >
        <div className="flex items-start gap-3">
          <AlertTriangle className="mt-0.5 size-4 shrink-0 text-danger" aria-hidden="true" />
          <div className="space-y-2 text-sm">
            <p>
              Все текущие данные — вакансии, статусы кандидата, журнал запусков модели — будут
              заменены содержимым копии от{' '}
              {restoreTarget ? formatDateTime(restoreTarget.created_at) : ''}.
            </p>
            <p className="text-muted">
              Перед восстановлением приложение само снимет копию текущего состояния, так что
              этот шаг можно будет отменить.
            </p>
          </div>
        </div>
      </Dialog>

      <Dialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        title="Удалить копию?"
        description={deleteTarget?.name}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>
              Отмена
            </Button>
            <Button variant="danger" onClick={handleDelete} disabled={remove.isPending}>
              {remove.isPending ? 'Удаляем…' : 'Удалить'}
            </Button>
          </>
        }
      >
        <p className="text-sm">
          Файл копии будет удалён с диска. Текущие данные не изменятся.
        </p>
      </Dialog>
    </div>
  )
}

function pluralBackups(count: number): string {
  const mod10 = count % 10
  const mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) {
    return 'копия'
  }
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) {
    return 'копии'
  }
  return 'копий'
}
