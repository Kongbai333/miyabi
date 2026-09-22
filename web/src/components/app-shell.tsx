import type { PropsWithChildren } from 'react'
import { useRouterState } from '@tanstack/react-router'
import {
  CompassIcon,
  FilmIcon,
  ListChecksIcon,
  MagnetIcon,
  SearchIcon,
  SettingsIcon
} from 'lucide-react'

import { FloatingNav, type FloatingNavItem } from '@/components/floating-nav'
import { Toaster } from '@/components/ui/sonner'
import { lastLibrarySearch } from '@/features/library/last-view'
import { MoviePlaybackNavAction } from '@/features/movie-detail/playback-nav-action'
import { TaskEventsProvider } from '@/features/tasks/task-events'
import { TaskNotifications } from '@/features/tasks/task-notifications'
import { PlayerDialog } from '@/features/player/player-dialog'

const NAV_ITEMS: FloatingNavItem[] = [
  { id: 'library', label: '媒体库', icon: FilmIcon, to: '/' },
  { id: 'discover', label: '发现', icon: CompassIcon, to: '/discover' },
  { id: 'search', label: '搜索', icon: SearchIcon, to: '/search' },
  { id: 'magnet', label: '磁力', icon: MagnetIcon, to: '/magnet' },
  { id: 'tasks', label: '任务', icon: ListChecksIcon, to: '/tasks' },
  { id: 'settings', label: '设置', icon: SettingsIcon, to: '/settings' }
]

export function AppShell({ children }: PropsWithChildren) {
  // The whole location is read, not just its path: a page that changes only its
  // search still has to redraw the nav item that carries that search.
  const location = useRouterState({ select: state => state.location })
  const pathname = location.pathname
  const activeId = NAV_ITEMS.find(item =>
    item.to === '/' ? pathname === '/' : pathname.startsWith(item.to)
  )?.id
  // The library is its own authority while it is open; from anywhere else the nav
  // returns to the page and filters the user left behind.
  const librarySearch = pathname === '/' ? location.search : lastLibrarySearch()
  const items = NAV_ITEMS.map(item => (item.to === '/' ? { ...item, search: librarySearch } : item))

  return (
    <TaskEventsProvider>
      <div className="relative min-h-dvh">
        <FloatingNav items={items} activeId={activeId}>
          <MoviePlaybackNavAction />
        </FloatingNav>
        {children}
        <PlayerDialog />
        <Toaster position="top-right" closeButton duration={6000} />
        <TaskNotifications />
      </div>
    </TaskEventsProvider>
  )
}
