import { LoaderCircleIcon, MagnetIcon, PlugZapIcon } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import {
  type MagnetSettings,
  type MagnetTestResult,
  useMagnetSettings,
  useTestMagnet,
  useUpdateMagnetSettings
} from '@/api/magnet'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { SettingRow, SettingsSection } from './shared'

const EMPTY: MagnetSettings = { url: '', token: '' }

// The token is stored and shown as typed: this app runs for one operator behind
// its own access password, so the field is plain text rather than a secret the
// backend refuses to hand back.
export function MagnetSection() {
  const settings = useMagnetSettings()
  const update = useUpdateMagnetSettings()
  const test = useTestMagnet()
  const [draft, setDraft] = useState<MagnetSettings | null>(null)
  const [result, setResult] = useState<MagnetTestResult | null>(null)

  const saved = settings.data
  const value = draft ?? saved ?? EMPTY
  const dirty =
    draft !== null && (draft.url !== (saved?.url ?? '') || draft.token !== (saved?.token ?? ''))
  const busy = settings.isPending || update.isPending || test.isPending
  const ready = value.token.trim() !== ''

  function edit(patch: Partial<MagnetSettings>) {
    setResult(null)
    setDraft(current => ({ ...(current ?? saved ?? EMPTY), ...patch }))
  }

  function save() {
    update.mutate(value, {
      onSuccess: () => {
        setDraft(null)
        toast.success('磁力服务设置已保存')
      },
      onError: error => toast.error('无法保存磁力服务设置', { description: error.message })
    })
  }

  function runTest() {
    test.mutate(undefined, {
      onSuccess: tools => {
        setResult(tools)
        toast.success(`连接成功，可用工具 ${tools.tools.length} 个`)
      },
      onError: error => {
        setResult(null)
        toast.error('无法连接磁力服务', { description: error.message })
      }
    })
  }

  return (
    <SettingsSection icon={<MagnetIcon className="size-4" />} title="磁力">
      <SettingRow title="服务地址" description="提供磁力搜索的 MCP 服务。留空使用默认地址。">
        {settings.isPending ? (
          <Skeleton className="h-9 w-full sm:w-72" />
        ) : (
          <Input
            value={value.url}
            placeholder="https://magnet.kiteyuan.info/api/v1/mcp"
            spellCheck={false}
            disabled={update.isPending}
            onChange={event => edit({ url: event.target.value })}
            className="w-full sm:w-72"
          />
        )}
      </SettingRow>
      <SettingRow
        title="访问 token"
        description="服务方发放的 bearer token，与站点登录是两套凭据。它在生成或重置后只显示一次，先复制再回来填；此页明文保存并回显。"
      >
        {settings.isPending ? (
          <Skeleton className="h-9 w-full sm:w-72" />
        ) : (
          <Input
            value={value.token}
            placeholder="mcp_..."
            spellCheck={false}
            autoComplete="off"
            disabled={update.isPending}
            onChange={event => edit({ token: event.target.value })}
            className="w-full font-mono text-xs sm:w-72"
          />
        )}
      </SettingRow>
      <SettingRow
        title="连接测试"
        description={
          result ? `可用工具：${result.tools.join('、')}` : '向服务发送一次握手并列出它提供的工具。'
        }
      >
        <div className="flex items-center gap-2">
          <Button type="button" size="sm" disabled={busy || !dirty} onClick={save}>
            {update.isPending ? <LoaderCircleIcon className="animate-spin" /> : null}
            保存
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={busy || !ready}
            onClick={runTest}
          >
            {test.isPending ? <LoaderCircleIcon className="animate-spin" /> : <PlugZapIcon />}
            测试连接
          </Button>
        </div>
      </SettingRow>
      {settings.isError ? (
        <InlineError onRetry={() => void settings.refetch()} retrying={settings.isFetching}>
          无法读取磁力服务设置，请稍后重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}
