import { LoaderCircleIcon } from 'lucide-react'
import { useEffect, useState, type FormEvent, type PropsWithChildren } from 'react'
import { useQueryClient } from '@tanstack/react-query'

import { useAccessGateLogin, useAccessGateSession } from '@/api/auth'
import { setUnauthorizedHandler } from '@/api/client'
import { InlineError } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

export function AccessGate({ children }: PropsWithChildren) {
  const queryClient = useQueryClient()
  const [password, setPassword] = useState('')
  const session = useAccessGateSession(true)
  const login = useAccessGateLogin()

  // Any 401 means the cookie lapsed mid-session; drop the cached session so
  // the form replaces whatever page the visitor was reading.
  useEffect(() => {
    setUnauthorizedHandler(() =>
      queryClient.setQueryData(['auth', 'session'], { authenticated: false, enabled: true })
    )
  }, [queryClient])
  // The cookie is the single source of truth, so a new tab, a reopened
  // bookmark and an expired session all resolve the same way.
  if (session.isPending) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-muted px-4 py-10">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" />
          正在检查访问配置
        </div>
      </main>
    )
  }

  if (session.isError) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-muted px-4 py-10">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <CardTitle>访问 Miyabi</CardTitle>
          </CardHeader>
          <CardContent>
            <InlineError
              className="flex-col text-center"
              onRetry={() => void session.refetch()}
              retrying={session.isFetching}
            >
              无法读取访问配置，请确认后端服务已启动。
            </InlineError>
          </CardContent>
        </Card>
      </main>
    )
  }

  if (session.data.authenticated || session.data.enabled === false) return children

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    login.mutate(password, { onSuccess: () => setPassword('') })
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-muted px-4 py-10">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle>访问 Miyabi</CardTitle>
          <CardDescription>请输入部署时配置的访问密码</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <Input
              type="password"
              value={password}
              placeholder="访问密码"
              autoComplete="current-password"
              autoFocus
              disabled={login.isPending}
              onChange={event => setPassword(event.target.value)}
            />
            {login.error ? <InlineError>{login.error.message}</InlineError> : null}
            <Button type="submit" className="w-full" disabled={!password || login.isPending}>
              {login.isPending ? <LoaderCircleIcon className="size-4 animate-spin" /> : null}
              进入
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  )
}
