import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import {
  isHLSProvider,
  MediaPlayer,
  MediaProvider,
  useMediaPlayer,
  useMediaState,
  type MediaPlayerInstance
} from '@vidstack/react'
import { DefaultVideoLayout } from '@vidstack/react/player/layouts/default'

import { usePlayback, usePlayFiles, type PlayFiles } from '@/api/play'
import type { WatchHistoryScope, WatchResume } from '@/api/watch-history'
import { watchResumePosition } from '@/lib/watch-progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { PlayerControlsVisibility } from './controls-visibility'
import { playerIcons } from './icons'
import {
  PlayerCloseButton,
  PlayerError,
  PlayerLoading,
  PlayerLoadingIndicator,
  PlayerTitle
} from './player-status'
import { playerTranslations } from './translations'
import { PlayerTimeSlider, PlayerVolumeSlider } from './sliders'
import { useHoldSpeed } from './use-hold-speed'
import { useWatchProgress } from './use-watch-progress'
import { watchSessions } from './watch-progress'

import '@vidstack/react/player/styles/default/theme.css'
import '@vidstack/react/player/styles/default/layouts/video.css'
import './player.css'

export default function MoviePlayer({ movieID }: { movieID: number }) {
  const openingID = useId()
  const files = usePlayFiles(movieID, openingID)

  if (files.isPending) return <PlayerLoading />
  if (files.isError) {
    return <PlayerError error={files.error} onRetry={() => void files.refetch()} />
  }
  return (
    <MoviePlayback
      key={files.dataUpdatedAt}
      movieID={movieID}
      openingID={openingID}
      files={files.data}
      onRetry={() => void files.refetch()}
    />
  )
}

function MoviePlayback({
  movieID,
  openingID,
  files,
  onRetry
}: {
  movieID: number
  openingID: string
  files: PlayFiles
  onRetry: () => void
}) {
  // Freeze file selection and resume for this opening. A late watch write must
  // never switch files or seek backwards after playback has already started.
  const [{ file, history, source, title }] = useState(() => {
    const history = watchSessions.resume(movieID, files.source, files.resume)
    return {
      history,
      source: files.source,
      title: files.title,
      file: files.files.find(item => item.id === history?.file_id) ?? files.files[0]
    }
  })
  if (!file) {
    return (
      <PlayerError title={title} message="没有可播放的文件，请重新扫描媒体库。" onRetry={onRetry} />
    )
  }

  return (
    <PlaybackPlayer
      title={title}
      movieID={movieID}
      openingID={openingID}
      source={source}
      fileID={file.id}
      history={history}
    />
  )
}

function PlaybackPlayer({
  title,
  movieID,
  openingID,
  source: watchSource,
  fileID,
  history
}: {
  title: string
  movieID: number
  openingID: string
  source: WatchHistoryScope
  fileID: string
  history?: WatchResume
}) {
  const playback = usePlayback(fileID, openingID)
  const [selectedSrc, setSelectedSrc] = useState<string>()
  const [player, setPlayer] = useState<MediaPlayerInstance | null>(null)
  const holdSpeed = useHoldSpeed(player)
  const [failed, setFailed] = useState(false)
  const [autoPlay, setAutoPlay] = useState(true)
  const initialPosition = watchResumePosition(history, fileID)
  const position = useRef(initialPosition)
  const resumeTime = useRef<number | null>(initialPosition)
  const playing = useRef(false)
  const progress = useWatchProgress(movieID, watchSource, fileID, history)
  const sources = playback.data?.sources ?? []
  const source = sources.find(item => item.src === selectedSrc) ?? sources[0]
  const loading = playback.isPending || playback.isFetching

  const retry = () => {
    resumeTime.current = position.current
    setFailed(false)
    setAutoPlay(true)
    void playback.refetch()
  }

  return (
    <MediaPlayer
      ref={setPlayer}
      className="miyabi-player dark"
      title={title}
      src={loading || playback.isError || failed ? undefined : source}
      viewType="video"
      streamType="on-demand"
      autoPlay={autoPlay}
      playsInline
      onCanPlay={() => {
        if (player && resumeTime.current !== null) {
          const target = Math.min(resumeTime.current, Math.max(0, player.duration - 1))
          if (target > 0) player.remoteControl.seek(target)
          else resumeTime.current = null
        }
      }}
      onTimeUpdate={detail => {
        if (resumeTime.current !== null || !player || !playing.current) return
        position.current = detail.currentTime
        progress?.update(detail.currentTime, player.duration)
      }}
      onPlaying={() => {
        playing.current = true
      }}
      onSeeked={currentTime => {
        resumeTime.current = null
        position.current = currentTime
        if (player) progress?.update(currentTime, player.duration)
        void progress?.flush(true)
      }}
      onPause={() => {
        playing.current = false
        if (player && resumeTime.current === null)
          progress?.update(position.current, player.duration)
        void progress?.flush(true)
      }}
      onEnded={() => {
        playing.current = false
        if (player) progress?.update(player.duration, player.duration)
        void progress?.flush(true)
      }}
      onError={() => {
        resumeTime.current = position.current
        setFailed(true)
      }}
      onProviderChange={provider => {
        if (isHLSProvider(provider)) provider.library = () => import('hls.js')
      }}
    >
      <PlayerControlsVisibility />
      <MediaProvider />
      {holdSpeed ? <div className="miyabi-player-feedback">倍速播放中</div> : null}
      {loading || playback.isError || failed ? (
        <div className="absolute inset-0 z-20 cursor-auto">
          {loading ? (
            <PlayerLoading title={title} />
          ) : (
            <PlayerError
              title={title}
              error={playback.error ?? undefined}
              message="播放中断，请重新加载播放地址。"
              onRetry={retry}
            />
          )}
        </div>
      ) : (
        <PlayerReady title={title}>
          <DefaultVideoLayout
            icons={playerIcons}
            translations={playerTranslations}
            colorScheme="dark"
            seekStep={5}
            noModal
            slots={{
              bufferingIndicator: null,
              googleCastButton: null,
              timeSlider: <PlayerTimeSlider />,
              volumeSlider: <PlayerVolumeSlider />,
              topControlsGroupStart: <PlayerTitle title={title} />,
              topControlsGroupEnd: <PlayerCloseButton />,
              chapterTitle: <div className="vds-controls-spacer" />,
              beforeSettingsMenu:
                sources.length > 1 && source ? (
                  <PlaybackQualitySelect
                    player={player}
                    value={source.src}
                    onValueChange={value => {
                      resumeTime.current = position.current
                      setAutoPlay(!player?.paused)
                      setSelectedSrc(value)
                    }}
                  >
                    {sources.map(item => (
                      <SelectItem key={item.src} value={item.src}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </PlaybackQualitySelect>
                ) : null
            }}
          >
            <PlayerBufferingIndicator />
          </DefaultVideoLayout>
        </PlayerReady>
      )}
    </MediaPlayer>
  )
}

function PlayerReady({ title, children }: { title: string; children: ReactNode }) {
  const canPlay = useMediaState('canPlay')
  const player = useMediaPlayer()
  const focused = useRef(false)

  useEffect(() => {
    if (!canPlay || !player?.el || focused.current) return
    focused.current = true
    player.el.focus({ preventScroll: true })
  }, [canPlay, player])

  if (!canPlay) {
    return (
      <div className="absolute inset-0 z-20 cursor-auto">
        <PlayerLoading title={title} />
      </div>
    )
  }

  return children
}

function PlayerBufferingIndicator() {
  const waiting = useMediaState('waiting')

  if (!waiting) return null

  return (
    <div className="pointer-events-none absolute inset-0 z-20 grid place-items-center">
      <div className="rounded-full bg-background/75 px-4 py-2.5 backdrop-blur-xl">
        <PlayerLoadingIndicator message="正在缓冲…" />
      </div>
    </div>
  )
}

function PlaybackQualitySelect({
  player,
  value,
  onValueChange,
  children
}: {
  player: MediaPlayerInstance | null
  value: string
  onValueChange: (value: string) => void
  children: ReactNode
}) {
  return (
    <Select
      value={value}
      onValueChange={onValueChange}
      onOpenChange={open => {
        if (open) player?.controls.pause()
        else player?.controls.resume()
      }}
    >
      <SelectTrigger size="sm" className="max-w-full min-w-0 shrink-0 px-2 text-xs">
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        container={player?.el}
        position="popper"
        align="end"
        side="top"
        collisionBoundary={player?.el}
        collisionPadding={12}
        className="max-w-[min(32rem,var(--radix-select-content-available-width))] bg-popover/90 backdrop-blur-xl"
      >
        {children}
      </SelectContent>
    </Select>
  )
}
