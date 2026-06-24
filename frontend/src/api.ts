import type {
  BootstrapResponse,
  PaymentScenario,
  RuleInput,
  RuleVersion,
  User,
  ViolationInput,
} from './types'

const API_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

async function request<T>(path: string, options: RequestInit = {}, userId?: string): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
      ...(userId ? { 'X-User-ID': userId } : {}),
    },
  })

  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null
    throw new Error(payload?.error ?? `Request failed with status ${response.status}`)
  }

  return (await response.json()) as T
}

export function getUsers(): Promise<User[]> {
  return request<User[]>('/api/users')
}

export function getBootstrap(userId: string): Promise<BootstrapResponse> {
  return request<BootstrapResponse>('/api/bootstrap', {}, userId)
}

export function getRules(): Promise<RuleVersion[]> {
  return request<RuleVersion[]>('/api/rules')
}

export function publishRules(userId: string, payload: RuleInput): Promise<RuleVersion> {
  return request<RuleVersion>('/api/rules', {
    method: 'POST',
    body: JSON.stringify(payload),
  }, userId)
}

export function submitViolation(userId: string, payload: ViolationInput) {
  return request<{ violation: unknown; notification: unknown }>('/api/violations', {
    method: 'POST',
    body: JSON.stringify(payload),
  }, userId)
}

export function payViolation(userId: string, violationId: string, scenario: PaymentScenario) {
  return request(`/api/violations/${violationId}/pay`, {
    method: 'POST',
    body: JSON.stringify({ scenario }),
  }, userId)
}
