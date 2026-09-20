import { useState } from 'react'
import { GlobeIcon, RefreshCwIcon } from 'lucide-react'
import { toast } from 'sonner'

import { useNetworkConfig, useTestNetwork, useUpdateNetworkConfig } from '@/api/network'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { SettingRow, SettingsSection } from './shared'

export function NetworkSection() {
  const network = useNetworkConfig()
  const updateConfig = useUpdateNetworkConfig()
  const testNetwork = useTestNetwork()

  const config = network.data
  const [userInput, setUserInput] = useState<string | null>(null)

  const url = userInput ?? config?.url ?? ''
  const isDirty = userInput !== null && userInput !== (config?.url ?? '')

  const isBusy = network.isLoading || updateConfig.isPending
  const isTesting = testNetwork.isPending
  const isEnabled = config?.enabled ?? false

  function handleToggle(enabled: boolean) {
    const targetUrl = normalizeProxyInput(url)
    updateConfig.mutate(
      { enabled, url: targetUrl },
      {
        onSuccess: () => {
          setUserInput(null)
          toast.success(enabled ? '已开启网络代理' : '已关闭网络代理')
        },
        onError: error => {
          const message = error instanceof Error ? error.message : '修改代理状态失败'
          toast.error(friendlyErrorMessage(message))
        }
      }
    )
  }

  function handleSave() {
    if (!isDirty) return
    const targetUrl = normalizeProxyInput(url)
    updateConfig.mutate(
      { enabled: isEnabled, url: targetUrl },
      {
        onSuccess: () => {
          setUserInput(null)
          toast.success('代理地址已保存')
        },
        onError: error => {
          const message = error instanceof Error ? error.message : '保存代理地址失败'
          toast.error(friendlyErrorMessage(message))
        }
      }
    )
  }

  function handleTest() {
    const targetUrl = normalizeProxyInput(url)
    if (!targetUrl) {
      toast.error('未提供有效的代理地址')
      return
    }
    testNetwork.mutate(
      { enabled: true, url: targetUrl },
      {
        onSuccess: res => {
          if (res.javdb.available || res.javbus.available) {
            toast.success('代理连接成功')
          } else {
            toast.error('代理连接失败')
          }
        },
        onError: () => {
          toast.error('代理连接失败')
        }
      }
    )
  }

  return (
    <SettingsSection icon={<GlobeIcon className="size-4" />} title="网络代理">
      <SettingRow title="代理服务" description="仅为 JavDB 与 JavBus 提供网络代理" inline>
        <Switch checked={isEnabled} onCheckedChange={handleToggle} />
      </SettingRow>

      {isEnabled ? (
        <>
          <SettingRow title="代理地址" description="支持 HTTP、HTTPS 与 SOCKS5 代理协议">
            <div className="flex w-full items-center gap-2 sm:w-auto">
              <Input
                type="text"
                value={url}
                placeholder="http://127.0.0.1:7890"
                className="w-full sm:w-80"
                disabled={isBusy || network.isError}
                onChange={e => {
                  setUserInput(e.target.value)
                }}
                onKeyDown={e => {
                  if (e.key === 'Enter') handleSave()
                }}
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isBusy || !isDirty || network.isError}
                onClick={handleSave}
              >
                保存
              </Button>
            </div>
          </SettingRow>

          <SettingRow title="连通性测试" description="测试当前填写的代理地址网络连通状态" inline>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={isBusy || isTesting || network.isError}
              onClick={handleTest}
            >
              <RefreshCwIcon className={cn('size-3.5', isTesting && 'animate-spin')} />
              测试连接
            </Button>
          </SettingRow>
        </>
      ) : null}

      {network.isError ? (
        <InlineError>
          后端服务暂不可用，无法读取网络设置。请检查后端服务状态后刷新重试。
        </InlineError>
      ) : null}
    </SettingsSection>
  )
}

function normalizeProxyInput(input: string): string {
  const trimmed = input.trim()
  if (!trimmed) return ''
  if (!/^[a-zA-Z][a-zA-Z0-9+.-]*:\/\//.test(trimmed)) {
    return `http://${trimmed}`
  }
  return trimmed
}

function friendlyErrorMessage(raw: string): string {
  if (raw.includes('scheme and host')) {
    return '代理地址必须包含协议（如 http://）与主机地址'
  }
  if (raw.includes('must be http, https or socks5')) {
    return '代理协议仅支持 http://、https:// 或 socks5://'
  }
  return raw
}
