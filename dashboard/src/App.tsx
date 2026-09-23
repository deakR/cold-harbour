import { useEffect, useState, type FormEvent } from 'react'

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

export default function App() {
  const [view, setView] = useState<View>('jobs')
  const [apiKey, setApiKey] = useState('')
  const [jobType, setJobType] = useState('redact')
  const [input, setInput] = useState('Call 415-555-0199 before noon.')
  const [jobs, setJobs] = useState<JobRow[]>([])
  const [selected, setSelected] = useState<JobDetail | null>(null)
  const [events, setEvents] = useState<LiveEvent[]>([])
  const [nodes, setNodes] = useState<SentinelNode[]>([])
  const [link, setLink] = useState('')
  const [notice, setNotice] = useState('')

  async function loadJobs() {
    const res = await fetch('/v1/jobs', { headers: { 'X-API-Key': apiKey } })
    if (!res.ok) {
      setNotice(`job list ${res.status}`)
      return
    }
    setJobs(await res.json())
  }

  async function loadNodes() {
    const res = await fetch('/v1/sentinel/nodes', { headers: { 'X-API-Key': apiKey } })
    if (!res.ok) {
      setNotice(`sentinel nodes ${res.status}`)
      return
    }
    setNodes(await res.json())
  }

  useEffect(() => {
    if (!apiKey) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/v1/ws/events?apiKey=${encodeURIComponent(apiKey)}`)
    ws.onmessage = (ev) => {
      const msg = JSON.parse(ev.data) as LiveEvent
      setEvents((prev) => [msg, ...prev].slice(0, 20))
      void loadJobs()
    }
    return () => ws.close()
  }, [apiKey])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setNotice('')
    const res = await fetch('/v1/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-API-Key': apiKey },
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
    const res = await fetch(`/v1/jobs/${jobId}`, { headers: { 'X-API-Key': apiKey } })
    if (!res.ok) {
      setNotice(`job ${res.status}`)
      return
    }
    setSelected(await res.json())
    setLink('')
  }

  async function makeLink() {
    if (!selected) return
    const expiresAt = new Date(Date.now() + 60 * 60 * 1000).toISOString()
    const res = await fetch(`/v1/jobs/${selected.jobId}/delivery-links`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-API-Key': apiKey },
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
    const from = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
    const to = new Date(Date.now() + 60 * 60 * 1000).toISOString()
    const url = `/v1/reports/compliance?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
    fetch(url, { headers: { 'X-API-Key': apiKey } })
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

  return (
    <main>
      <h1>Cold Harbour</h1>
      <nav>
        <button type="button" onClick={() => setView('jobs')}>Jobs</button>
        <button type="button" onClick={() => { setView('sentinel'); void loadNodes() }}>Sentinel</button>
      </nav>
      <label>
        API key
        <input value={apiKey} onChange={(e) => setApiKey(e.target.value)} />
      </label>
      <p>{notice}</p>
      {view === 'jobs' && (
        <>
          <form onSubmit={submit}>
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
