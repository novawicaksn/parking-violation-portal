export type Role = 'officer' | 'member'

export type ViolationType =
  | 'expired_meter'
  | 'no_parking_zone'
  | 'blocking_hydrant'
  | 'disabled_spot'

export type ViolationStatus = 'unpaid' | 'paid' | 'failed'
export type PaymentScenario = 'success' | 'failed'
export type PaymentStatus = 'paid' | 'failed'

export interface TimeWindow {
  start: string
  end: string
  multiplier: number
}

export interface RepeatMultipliers {
  zero: number
  one: number
  two_plus: number
}

export interface FineRules {
  base_amounts: Record<ViolationType, number>
  time_windows: TimeWindow[]
  repeat_multipliers: RepeatMultipliers
}

export interface User {
  id: string
  name: string
  role: Role
  plate?: string
  balance_cents: number
  created_at: string
}

export interface RuleVersion {
  id: string
  version: number
  active: boolean
  rules: FineRules
  created_at: string
}

export interface PaymentRecord {
  id: string
  violation_id: string
  member_user_id: string
  amount_cents: number
  scenario: PaymentScenario
  status: PaymentStatus
  transaction_id: string
  created_at: string
}

export interface Violation {
  id: string
  plate: string
  violation_type: ViolationType
  location: string
  occurred_at: string
  photo_data: string
  fine_cents: number
  rule_version_id: string
  rule_version_number: number
  rule_snapshot: FineRules
  prior_unpaid_count: number
  time_multiplier: number
  repeat_multiplier: number
  status: ViolationStatus
  owner_user_id?: string | null
  payment_transaction?: PaymentRecord | null
  created_at: string
}

export interface Notification {
  id: string
  violation_id: string
  user_id: string
  kind: string
  message: string
  status: string
  created_at: string
}

export interface BootstrapResponse {
  current_user: User
  users: User[]
  active_rule: RuleVersion
  violations: Violation[]
  payments: PaymentRecord[]
  notifications: Notification[]
}

export interface ViolationInput {
  plate: string
  violation_type: ViolationType
  location: string
  occurred_at: string
  photo_data: string
}

export interface RuleInput {
  base_amounts: Record<ViolationType, number>
  time_windows: TimeWindow[]
  repeat_multipliers: RepeatMultipliers
}
