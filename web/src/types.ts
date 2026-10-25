// Shared wire types mirrored from the Go backend (internal/types + engine).
export type Phase = 'idle' | 'running' | 'paused' | 'ended'

export interface Question {
  id: number
  text: string
  duration_sec: number
  started_at_unix: number
}

export interface LeaderRow {
  username: string
  points: number
  correct: number
  wrong: number
  streak: number
  rank: number
}

export interface State {
  game: Phase
  mode: string
  question: Question | null
  time_left: number
  round: number
  round_max: number
  viewers: number
  connected: boolean
  provider: string
  live_title: string
  room_id: string
  scores: LeaderRow[]
}

export interface AnswerResult {
  username: string
  correct: boolean
  answer: string
  points: number
  bonus: number
  streak: number
  question_id: number
  now: number
}

export interface GiftRaised {
  raised: number
  target: number
}

export interface RawEvent {
  kind: string
  username: string
  text?: string
  gift_name?: string
  gift_value?: number
  like_count?: number
  viewers?: number
}

export interface WSMessage {
  type: string
  // data is one of: State / LeaderRow[] / Question / AnswerResult / GiftRaised / RawEvent / ProviderStatus
  data: any
}

export interface EngineConfig {
  difficulty: 'easy' | 'medium' | 'hard'
  timer_sec: number
  base_points: number
  speed_bonus: boolean
  combo_bonus: boolean
  round_total: number
  gift_target: number
}

export interface ProviderStatus {
  type: string
  connected: boolean
  error?: string
  username?: string
  room_id?: string
}
