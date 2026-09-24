import { useCallback, useEffect, useState, type FormEvent } from 'react'

type JobRow = { jobId: string; status: string; acceptedAt: string }
type JobDetail = {
  jobId: string
  status: string
  result?: unknown
  signature?: string
  signingKeyId?: string
}
type LiveEvent = { tenantId: string; jobId: string; status: string }
type SentinelNode = {
  nodeId: string
  lastSeen: string
  policyVersion: number
  counts: Record<string, number>
  held: number
  dropped: number
  failOpen: number
}

type View = 'jobs' | 'sentinel'

const maskSample = '{"document":{"email":"a@b.com","name":"Ann"},"fields":["email"]}'

function sessionHeaders(csrfToken: string | null, method: string, extra?: HeadersInit): Headers {
  const headers = new Headers(extra)
  const m = method.toUpperCase()
  if (csrfToken && m !== 'GET' && m !== 'HEAD') {
    headers.set('X-CSRF-Token', csrfToken)
  }
  return headers
}

async function api(
  path: string,
  csrfToken: string | null,
  init: RequestInit = {},
): Promise<Response> {
  const method = init.method ?? 'GET'
  return fetch(path, {
    ...init,
    credentials: 'include',
    headers: sessionHeaders(csrfToken, method, init.headers),
  })
}

export default function App() {
  const [view, setView] = useState<View>('jobs')
  const [csrfToken, setCsrfToken] = useState<string | null>(null)
  const [jobType, setJobType] = useState('redact')
  const [input, setInput] = useState('Call 415-555-0199 before noon.')
  const [jobs, setJobs] = useState<JobRow[]>([])
  const [selected, setSelected] = useState<JobDetail | null>(null)
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [nodes, setNodes] = useState<SentinelNode[]>([])
  const [link, setLink] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    void (async () => {
      const res = await fetch('/v1/session', { credentials: 'include' })
      if (!res.ok) return
      const body = (await res.json()) as { csrfToken?: string }
      if (body.csrfToken) setCsrfToken(body.csrfToken)
    })()
  }, [])

  const loadJobs = useCallback(async () => {
    if (!csrfToken) return
    const res = await api('/v1/jobs', csrfToken)
    if (!res.ok) {
      setNotice(`job list ${res.status}`)
      return
    }
    setJobs(await res.json())
  }, [csrfToken])

  async function loadNodes() {
    if (!csrfToken) return
    const res = await api('/v1/sentinel/nodes', csrfToken)
    if (!res.ok) {
      setNotice(`sentinel nodes ${res.status}`)
      return
    }
    setNodes(await res.json())
  }

  useEffect(() => {
    if (!csrfToken) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/v1/ws/events`)
    ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data) as LiveEvent
      setEvents((prev) => [msg, ...prev].slice(0, 20))
      void loadJobs()
    }
    return () => ws.close()
  }, [csrfToken, loadJobs])

  async function login(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setNotice('')
    const form = e.currentTarget
    const apiKey = String(new FormData(form).get('apiKey') ?? '')
    const res = await fetch('/v1/session/login', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ apiKey }),
    })
    form.reset()
    const body = (await res.json()) as { csrfToken?: string; error?: string }
    if (!res.ok || !body.csrfToken) {
      setNotice(body.error ? JSON.stringify(body) : `login ${res.status}`)
      return
    }
    setCsrfToken(body.csrfToken)
  }

  async function logout() {
    setNotice('')
    if (csrfToken) {
      await api('/v1/session/logout', csrfToken, { method: 'POST' })
    }
    setCsrfToken(null)
    setJobs([])
    setSelected(null)
    setEvents([])
    setNodes([])
    setLink('')
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!csrfToken) return
    setNotice('')
    const res = await api('/v1/jobs', csrfToken, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ input, jobType }),
    })
    const body = await res.json()
    if (!res.ok) {
      setNotice(JSON.stringify(body))
      return
    }
    setNotice(`queued ${body.jobId}`)
    await loadJobs()
  }

  async function openJob(jobId: string) {
    if (!csrfToken) return
    const res = await api(`/v1/jobs/${jobId}`, csrfToken)
    if (!res.ok) {
      setNotice(`job ${res.status}`)
      return
    }
    setSelected(await res.json())
    setLink('')
  }

  async function makeLink() {
    if (!selected || !csrfToken) return
    const expiresAt = new Date(Date.now() + 60 * 60 * 1000).toISOString()
    const res = await api(`/v1/jobs/${selected.jobId}/delivery-links`, csrfToken, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ expiresAt, maxViews: '3' }),
    })
    const body = await res.json()
    if (!res.ok) {
      setNotice(JSON.stringify(body))
      return
    }
    setLink(`${location.origin}/d/${body.token}`)
  }

  function downloadReport() {
    if (!csrfToken) return
    const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
    const to = new Date(Date.now() + 60 * 60 * 1000).toISOString()
    const url = `/v1/reports/compliance?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
    void api(url, csrfToken)
      .then((res) => res.blob())
      .then((blob) => {
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = 'compliance.csv'
        a.click()
      })
  }

  function formatCounts(counts: Record<string, number>) {
    const keys = Object.keys(counts).sort()
    if (keys.length === 0) return '-'
    return keys.map((k) => `${k}:${counts[k]}`).join(' ')
  }

  if (!csrfToken) {
    return (
      <main>
        <h1>Cold Harbour</h1>
        <p>{notice}</p>
        <form onSubmit={(e) => void login(e)}>
          <label>
            API key
            <input name="apiKey" type="password" autoComplete="off" required />
          </label>
          <button type="submit">Log in</button>
        </form>
      </main>
    )
  }

  return (
    <main>
      <h1>Cold Harbour</h1>
      <nav>
        <button type="button" onClick={() => setView('jobs')}>Jobs</button>
        <button type="button" onClick={() => { setView('sentinel'); void loadNodes() }}>Sentinel</button>
        <button type="button" onClick={() => void logout()}>Log out</button>
      </nav>
      <p>{notice}</p>
      {view === 'jobs' && (
        <>
          <form onSubmit={(e) => void submit(e)}>
            <label>
              Job type
              <select value={jobType} onChange={(e) => {
                setJobType(e.target.value)
                setInput(e.target.value === 'mask' ? maskSample : 'Call 415-555-0199 before noon.')
              }}>
                <option value="redact">redact</option>
                <option value="mask">mask</option>
              </select>
            </label>
            <label>
              Input
              <textarea value={input} onChange={(e) => setInput(e.target.value)} rows={4} />
            </label>
            <button type="submit">Submit job</button>
          </form>
          <h2>Live events</h2>
          <ul>
            {events.map((ev, i) => (
              <li key={`${ev.jobId}-${ev.status}-${i}`}>{ev.jobId} {ev.status}</li>
            ))}
          </ul>
          <h2>Jobs</h2>
          <button type="button" onClick={() => void loadJobs()}>Refresh</button>
          <table>
            <thead>
              <tr><th>Job</th><th>Status</th><th>Accepted</th></tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.jobId} onClick={() => void openJob(job.jobId)}>
                  <td>{job.jobId}</td>
                  <td>{job.status}</td>
                  <td>{job.acceptedAt}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {selected && (
            <section>
              <h2>Signed result</h2>
              <pre>{JSON.stringify(selected.result, null, 2)}</pre>
              <p>Signature {selected.signature}</p>
              <p>Key {selected.signingKeyId}</p>
              <button type="button" onClick={() => void makeLink()}>Delivery link</button>
              {link && <p><a href={link}>{link}</a></p>}
            </section>
          )}
          <button type="button" onClick={downloadReport}>Download compliance CSV</button>
        </>
      )}
      {view === 'sentinel' && (
        <section>
          <h2>Sentinel nodes</h2>
          <button type="button" onClick={() => void loadNodes()}>Refresh</button>
          <table>
            <thead>
              <tr>
                <th>Node</th>
                <th>Last seen</th>
                <th>Policy</th>
                <th>Counts</th>
                <th>Held</th>
                <th>Dropped</th>
                <th>Fail open</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((node) => (
                <tr key={node.nodeId}>
                  <td>{node.nodeId}</td>
                  <td>{node.lastSeen}</td>
                  <td>{node.policyVersion}</td>
                  <td>{formatCounts(node.counts ?? {})}</td>
                  <td>{node.held}</td>
                  <td>{node.dropped}</td>
                  <td>{node.failOpen}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
    </main>
  )
}
