import { AppPage } from '@/components/app-page'
import { PageHeader } from '@/components/page-header'
import { MagnetCollectionsCard } from './collections-card'
import { MagnetSearchCard } from './search-card'

export function MagnetPage() {
  return (
    <AppPage>
      <PageHeader title="磁力搜索" description="搜索磁力资源，查看详情和预览图" />
      <MagnetSearchCard />
      <MagnetCollectionsCard />
    </AppPage>
  )
}
