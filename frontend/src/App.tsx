import { type FormEvent, useEffect, useState } from 'react'
import {
  getBootstrap,
  getRules,
  getUsers,
  payViolation,
  publishRules,
  submitViolation,
} from './api'
import type {
  BootstrapResponse,
  PaymentScenario,
  RuleInput,
  RuleVersion,
  TimeWindow,
  User,
  Violation,
  ViolationType,
} from './types'

const violationTypes: { value: ViolationType; label: string }[] = [
  { value: 'expired_meter', label: 'Expired meter' },
  { value: 'no_parking_zone', label: 'No parking zone' },
  { value: 'blocking_hydrant', label: 'Blocking hydrant' },
  { value: 'disabled_spot', label: 'Disabled spot' },
]

const scenarioOptions: { value: PaymentScenario; label: string }[] = [
  { value: 'success', label: 'Success' },
  { value: 'failed', label: 'Failed' },
]

const formatCurrency = (value: number) => new Intl.NumberFormat('id-ID').format(value)
const formatDateTime = (value: string) => new Date(value).toLocaleString('en-ID', { dateStyle: 'medium', timeStyle: 'short' })

function toLocalDatetimeValue(date: Date) {
  const offset = date.getTimezoneOffset()
  const local = new Date(date.getTime() - offset * 60_000)
  return local.toISOString().slice(0, 16)
}

function emptyViolationForm() {
  return {
    plate: 'B1234CD',
    violation_type: 'expired_meter' as ViolationType,
    location: 'Jl. Sudirman 12',
    occurred_at: toLocalDatetimeValue(new Date()),
    photo_data: '',
  }
}

function cloneRuleInput(rule: RuleVersion['rules']): RuleInput {
  return {
    base_amounts: { ...rule.base_amounts },
    time_windows: rule.time_windows.map((window) => ({ ...window })),
    repeat_multipliers: { ...rule.repeat_multipliers },
  }
}

function App() {
  const [users, setUsers] = useState<User[]>([])
  const [selectedUserId, setSelectedUserId] = useState<string>(() => localStorage.getItem('parking-user-id') ?? '')
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null)
  const [rules, setRules] = useState<RuleVersion[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [violationForm, setViolationForm] = useState(emptyViolationForm())
  const [ruleForm, setRuleForm] = useState<RuleInput | null>(null)
  const [payScenarios, setPayScenarios] = useState<Record<string, PaymentScenario>>({})
  const [notice, setNotice] = useState<string | null>(null)

  useEffect(() => {
    getUsers()
      .then((nextUsers) => {
        setUsers(nextUsers)
        if (!selectedUserId && nextUsers.length > 0) {
          setSelectedUserId(nextUsers[0].id)
        }
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!selectedUserId) {
      return
    }
    localStorage.setItem('parking-user-id', selectedUserId)
    setBusy(true)
    Promise.all([getBootstrap(selectedUserId), getRules()])
      .then(([dashboard, nextRules]) => {
        setBootstrap(dashboard)
        setRules(nextRules)
        setRuleForm(cloneRuleInput(dashboard.active_rule.rules))
        setError(null)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusy(false))
  }, [selectedUserId])

  useEffect(() => {
    if (!bootstrap?.current_user) {
      return
    }
    setViolationForm((current) => ({
      ...current,
      plate: bootstrap.current_user.role === 'member' && bootstrap.current_user.plate ? bootstrap.current_user.plate : current.plate,
    }))
  }, [bootstrap?.current_user?.id])

  const currentUser = bootstrap?.current_user ?? null
  const isOfficer = currentUser?.role === 'officer'
  const isMember = currentUser?.role === 'member'
  const activeRule = bootstrap?.active_rule ?? null
  const memberViolations = (bootstrap?.violations ?? []).filter((violation) => violation.owner_user_id === currentUser?.id)
  const unpaidViolations = memberViolations.filter((violation) => violation.status === 'unpaid')
  const paidViolations = memberViolations.filter((violation) => violation.status === 'paid')
  const latestViolations = bootstrap?.violations ?? []

  const refresh = async () => {
    if (!selectedUserId) return
    const [dashboard, nextRules] = await Promise.all([getBootstrap(selectedUserId), getRules()])
    setBootstrap(dashboard)
    setRules(nextRules)
    setRuleForm(cloneRuleInput(dashboard.active_rule.rules))
  }

  const handleSubmitViolation = async (event: FormEvent) => {
    event.preventDefault()
    if (!currentUser) return
    setBusy(true)
    setNotice(null)
    setError(null)
    try {
      const result = await submitViolation(currentUser.id, {
        plate: violationForm.plate,
        violation_type: violationForm.violation_type,
        location: violationForm.location,
        occurred_at: new Date(violationForm.occurred_at).toISOString(),
        photo_data: violationForm.photo_data,
      })
      setNotice(`Violation ${String((result.violation as { id: string }).id)} created and notification recorded.`)
      setViolationForm((current) => ({ ...current, photo_data: '' }))
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to submit violation')
    } finally {
      setBusy(false)
    }
  }

  const handlePublishRules = async (event: FormEvent) => {
    event.preventDefault()
    if (!currentUser || !ruleForm) return
    setBusy(true)
    setNotice(null)
    setError(null)
    try {
      await publishRules(currentUser.id, ruleForm)
      setNotice('New active rule version published.')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to publish rules')
    } finally {
      setBusy(false)
    }
  }

  const handlePayViolation = async (violationId: string) => {
    if (!currentUser) return
    setBusy(true)
    setNotice(null)
    setError(null)
    try {
      await payViolation(currentUser.id, violationId, payScenarios[violationId] ?? 'success')
      setNotice('Payment processed.')
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to process payment')
    } finally {
      setBusy(false)
    }
  }

  const updateWindow = (index: number, field: keyof TimeWindow, value: string | number) => {
    if (!ruleForm) return
    const next = ruleForm.time_windows.map((window, windowIndex) =>
      windowIndex === index ? { ...window, [field]: value } : window,
    )
    setRuleForm({ ...ruleForm, time_windows: next })
  }

  const updateBaseAmount = (key: ViolationType, value: string) => {
    if (!ruleForm) return
    setRuleForm({
      ...ruleForm,
      base_amounts: {
        ...ruleForm.base_amounts,
        [key]: Number(value),
      },
    })
  }

  const updateMultiplier = (key: 'zero' | 'one' | 'two_plus', value: string) => {
    if (!ruleForm) return
    setRuleForm({
      ...ruleForm,
      repeat_multipliers: {
        ...ruleForm.repeat_multipliers,
        [key]: Number(value),
      },
    })
  }

  const selectAccount = (userId: string) => {
    setSelectedUserId(userId)
    setNotice(null)
    setError(null)
  }

  if (loading) {
    return <div className="shell loading-state">Loading portal...</div>
  }

  if (error && users.length === 0) {
    return (
      <div className="shell landing-shell">
        <section className="hero-card">
          <p className="eyebrow">Parking Violation Portal</p>
          <h1>Backend connection problem.</h1>
          <p className="hero-copy">{error}</p>
          <p className="muted-text">
            Make sure PostgreSQL is running, the backend is listening on the API URL used by the frontend, and seeded users exist.
          </p>
        </section>
      </div>
    )
  }

  if (users.length === 0) {
    return <div className="shell">No seeded users found.</div>
  }

  if (!currentUser) {
    return (
      <div className="shell landing-shell">
        <section className="hero-card">
          <p className="eyebrow">Parking Violation Portal</p>
          <h1>One web app for officers and members.</h1>
          <p className="hero-copy">
            Pick an account to enter the workflow. Officers can issue violations and publish new fine rules.
            Members can review their invoices and pay from their account balance.
          </p>
          <div className="account-grid">
            {users.map((user) => (
              <button key={user.id} className="account-card" onClick={() => selectAccount(user.id)}>
                <span className="account-role">{user.role}</span>
                <strong>{user.name}</strong>
                <span>{user.plate ? `Plate ${user.plate}` : 'Officer account'}</span>
              </button>
            ))}
          </div>
        </section>
      </div>
    )
  }

  return (
    <div className="shell app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Tan Digital assignment</p>
          <h1>Parking Violation Portal</h1>
        </div>
        <div className="session-card">
          <label>
            Active account
            <select value={selectedUserId} onChange={(event) => selectAccount(event.target.value)}>
              {users.map((user) => (
                <option key={user.id} value={user.id}>
                  {user.name} · {user.role}
                </option>
              ))}
            </select>
          </label>
          <div className="session-meta">
            <span className={`role-badge ${currentUser.role}`}>{currentUser.role}</span>
            <strong>{currentUser.name}</strong>
            {currentUser.plate ? <span>Plate {currentUser.plate}</span> : null}
          </div>
        </div>
      </header>

      <section className="stats-grid">
        <StatCard label="Active rule version" value={String(activeRule?.version ?? '—')} detail={activeRule ? `Published ${formatDateTime(activeRule.created_at)}` : 'No rule loaded'} />
        <StatCard label="Open violations" value={String(bootstrap?.violations.filter((violation) => violation.status === 'unpaid').length ?? 0)} detail="Snapshot stored on each invoice" />
        <StatCard label="Account balance" value={currentUser.role === 'member' ? `IDR ${formatCurrency(currentUser.balance_cents)}` : 'Officer role'} detail={currentUser.role === 'member' ? 'Deducted only after successful charge' : 'Members see balance here'} />
      </section>

      {notice ? <div className="notice success">{notice}</div> : null}
      {error ? <div className="notice error">{error}</div> : null}
      {busy ? <div className="notice muted">Syncing with the backend...</div> : null}

      <main className="dashboard-grid">
        {isOfficer && activeRule && ruleForm ? (
          <>
            <section className="panel panel-wide">
              <div className="panel-header">
                <div>
                  <p className="eyebrow">Officer flow</p>
                  <h2>Submit a violation</h2>
                </div>
              </div>
              <form className="form-grid" onSubmit={handleSubmitViolation}>
                <label>
                  License plate
                  <input value={violationForm.plate} onChange={(event) => setViolationForm({ ...violationForm, plate: event.target.value.toUpperCase() })} />
                </label>
                <label>
                  Violation type
                  <select value={violationForm.violation_type} onChange={(event) => setViolationForm({ ...violationForm, violation_type: event.target.value as ViolationType })}>
                    {violationTypes.map((option) => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </select>
                </label>
                <label>
                  Location
                  <input value={violationForm.location} onChange={(event) => setViolationForm({ ...violationForm, location: event.target.value })} />
                </label>
                <label>
                  Timestamp
                  <input type="datetime-local" value={violationForm.occurred_at} onChange={(event) => setViolationForm({ ...violationForm, occurred_at: event.target.value })} />
                </label>
                <label className="full-width">
                  Photo
                  <input type="file" accept="image/*" onChange={async (event) => {
                    const file = event.target.files?.[0]
                    if (!file) return
                    const reader = new FileReader()
                    reader.onload = () => {
                      setViolationForm((current) => ({ ...current, photo_data: String(reader.result ?? '') }))
                    }
                    reader.readAsDataURL(file)
                  }} />
                </label>
                {violationForm.photo_data ? <img className="photo-preview" src={violationForm.photo_data} alt="Violation preview" /> : null}
                <button className="primary-button full-width" type="submit">Issue violation</button>
              </form>
            </section>

            <section className="panel panel-wide">
              <div className="panel-header">
                <div>
                  <p className="eyebrow">Rules</p>
                  <h2>Publish a new fine version</h2>
                </div>
              </div>
              <form className="rules-form" onSubmit={handlePublishRules}>
                <div className="rules-grid">
                  {violationTypes.map((option) => (
                    <label key={option.value}>
                      {option.label}
                      <input
                        type="number"
                        value={ruleForm.base_amounts[option.value]}
                        onChange={(event) => updateBaseAmount(option.value, event.target.value)}
                      />
                    </label>
                  ))}
                </div>
                <div className="rules-grid two-col">
                  {ruleForm.time_windows.map((window, index) => (
                    <div key={`${window.start}-${index}`} className="mini-card">
                      <label>
                        Window {index + 1} start
                        <input value={window.start} onChange={(event) => updateWindow(index, 'start', event.target.value)} />
                      </label>
                      <label>
                        Window {index + 1} end
                        <input value={window.end} onChange={(event) => updateWindow(index, 'end', event.target.value)} />
                      </label>
                      <label>
                        Multiplier
                        <input type="number" step="0.1" value={window.multiplier} onChange={(event) => updateWindow(index, 'multiplier', Number(event.target.value))} />
                      </label>
                    </div>
                  ))}
                </div>
                <div className="rules-grid three-col">
                  <label>
                    Repeat 0
                    <input type="number" step="0.1" value={ruleForm.repeat_multipliers.zero} onChange={(event) => updateMultiplier('zero', event.target.value)} />
                  </label>
                  <label>
                    Repeat 1
                    <input type="number" step="0.1" value={ruleForm.repeat_multipliers.one} onChange={(event) => updateMultiplier('one', event.target.value)} />
                  </label>
                  <label>
                    Repeat 2+
                    <input type="number" step="0.1" value={ruleForm.repeat_multipliers.two_plus} onChange={(event) => updateMultiplier('two_plus', event.target.value)} />
                  </label>
                </div>
                <button className="primary-button" type="submit">Publish version</button>
              </form>
            </section>
          </>
        ) : null}

        {isMember ? (
          <section className="panel panel-wide">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Member flow</p>
                <h2>Pay outstanding violations</h2>
              </div>
            </div>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Violation</th>
                    <th>Fine</th>
                    <th>Rule version</th>
                    <th>Status</th>
                    <th>Payment scenario</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {unpaidViolations.map((violation) => (
                    <tr key={violation.id}>
                      <td>
                        <strong>{violation.violation_type}</strong>
                        <div className="muted-text">{violation.location}</div>
                      </td>
                      <td>IDR {formatCurrency(violation.fine_cents)}</td>
                      <td>v{violation.rule_version_number}</td>
                      <td><StatusPill status={violation.status} /></td>
                      <td>
                        <select value={payScenarios[violation.id] ?? 'success'} onChange={(event) => setPayScenarios({ ...payScenarios, [violation.id]: event.target.value as PaymentScenario })}>
                          {scenarioOptions.map((option) => (
                            <option key={option.value} value={option.value}>{option.label}</option>
                          ))}
                        </select>
                      </td>
                      <td>
                        <button className="secondary-button" onClick={() => handlePayViolation(violation.id)}>Charge account</button>
                      </td>
                    </tr>
                  ))}
                  {unpaidViolations.length === 0 ? (
                    <tr><td colSpan={6} className="empty-state">No unpaid violations.</td></tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </section>
        ) : null}

        <section className="panel panel-wide">
          <div className="panel-header">
            <div>
              <p className="eyebrow">Transaction history</p>
              <h2>Violation snapshots and applied rules</h2>
            </div>
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Plate</th>
                  <th>Issued fine</th>
                  <th>Rule version</th>
                  <th>Status</th>
                  <th>Applied multipliers</th>
                  <th>Payment</th>
                </tr>
              </thead>
              <tbody>
                {latestViolations.map((violation) => (
                  <tr key={violation.id}>
                    <td>
                      <strong>{violation.plate}</strong>
                      <div className="muted-text">{formatDateTime(violation.occurred_at)}</div>
                    </td>
                    <td>IDR {formatCurrency(violation.fine_cents)}</td>
                    <td>
                      v{violation.rule_version_number}
                      <div className="muted-text">Snapshot preserved</div>
                    </td>
                    <td><StatusPill status={violation.status} /></td>
                    <td>
                      Time {violation.time_multiplier} × Repeat {violation.repeat_multiplier}
                      <div className="muted-text">Prior unpaid: {violation.prior_unpaid_count}</div>
                    </td>
                    <td>
                      {violation.payment_transaction ? (
                        <div>
                          <strong>{violation.payment_transaction.status}</strong>
                          <div className="muted-text">{violation.payment_transaction.transaction_id}</div>
                        </div>
                      ) : (
                        <span className="muted-text">No payment yet</span>
                      )}
                    </td>
                  </tr>
                ))}
                {latestViolations.length === 0 ? (
                  <tr><td colSpan={6} className="empty-state">No violations recorded yet.</td></tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </section>

        {isOfficer ? (
          <section className="panel panel-wide">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Rule versions</p>
                <h2>Published rule history</h2>
              </div>
            </div>
            <div className="version-list">
              {rules.map((rule) => (
                <article key={rule.id} className={`version-card ${rule.active ? 'active' : ''}`}>
                  <div className="version-header">
                    <strong>Version {rule.version}</strong>
                    {rule.active ? <span className="role-badge officer">active</span> : <span className="muted-chip">archived</span>}
                  </div>
                  <div className="version-body">
                    <span>Base expired meter: IDR {formatCurrency(rule.rules.base_amounts.expired_meter)}</span>
                    <span>Base no parking: IDR {formatCurrency(rule.rules.base_amounts.no_parking_zone)}</span>
                    <span>Night multiplier: {rule.rules.time_windows[1]?.multiplier ?? 1.5}</span>
                    <span>Repeat 2+: {rule.rules.repeat_multipliers.two_plus}</span>
                  </div>
                </article>
              ))}
            </div>
          </section>
        ) : null}

        {isMember ? (
          <section className="panel panel-wide">
            <div className="panel-header">
              <div>
                <p className="eyebrow">Member dashboard</p>
                <h2>Account summary</h2>
              </div>
            </div>
            <div className="summary-grid">
              <SummaryCard title="Balance" value={`IDR ${formatCurrency(currentUser.balance_cents)}`} />
              <SummaryCard title="Unpaid violations" value={String(unpaidViolations.length)} />
              <SummaryCard title="Paid violations" value={String(paidViolations.length)} />
            </div>
          </section>
        ) : null}
      </main>
    </div>
  )
}

function StatCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="stat-card">
      <span>{label}</span>
      <strong>{value}</strong>
      <p>{detail}</p>
    </div>
  )
}

function SummaryCard({ title, value }: { title: string; value: string }) {
  return (
    <div className="summary-card">
      <span>{title}</span>
      <strong>{value}</strong>
    </div>
  )
}

function StatusPill({ status }: { status: string }) {
  return <span className={`status-pill ${status}`}>{status}</span>
}

export default App
