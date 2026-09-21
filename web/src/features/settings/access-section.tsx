import { LoaderCircleIcon, LockIcon, LogOutIcon } from 'lucide-react'

import { useAccessGateLogout, useAccessGateSession } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { SettingRow, SettingsSection } from './shared'

export function AccessSection() {
  const session = useAccessGateSession(true)
  const logout = useAccessGateLogout()

  // Without a configured password there is no session to end.
  if (!session.data?.enabled) return null

  return (
    <SettingsSection icon={<LockIcon className="size-4" />} title="访问控制">
      <SettingRow title="访问密码" description="已开启门禁。所有接口都需要登录后才能访问。">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={logout.isPending}
          onClick={() => logout.mutate()}
        >
          {logout.isPending ? <LoaderCircleIcon className="animate-spin" /> : <LogOutIcon />}
          退出登录
        </Button>
      </SettingRow>
    </SettingsSection>
  )
}
