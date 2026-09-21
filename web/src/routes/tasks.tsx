import { createFileRoute } from '@tanstack/react-router'

import { TasksPage } from '@/features/tasks/page'

export const Route = createFileRoute('/tasks')({
  component: TasksPage
})
