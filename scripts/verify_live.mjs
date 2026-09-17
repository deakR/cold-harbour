// Node 22.12+. Uses real services only; never falls back to simulation.
// Creates one verification job and retains its audit record.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';

const base = new URL(process.env.BASE_URL || 'http://localhost:3000');
const headers = process.env.API_KEY
  ? { 'X-API-Key': process.env.API_KEY }
  : { 'X-Context-Clearance': 'INNIE' };
const events = [];
let socket;
async function request(path, options = {}) {
  const response = await fetch(new URL(path, base), {
    ...options, headers: { ...headers, ...options.headers },
    signal: AbortSignal.timeout(10000),
  });
  assert(response.ok, `${path}: HTTP ${response.status}`);
  return response;
}
try {
  const html = await (await request('/')).text();
  assert(html.includes('id="root"'), 'Dashboard root missing');
  const health = await (await request('/actuator/health')).json();
  assert.equal(health.status, 'UP');
  const metrics = await (await request('/actuator/prometheus')).text();
  assert(metrics.includes('jvm_memory_used_bytes'), 'JVM metrics missing');
  console.log('PASS dashboard HTML, proxied health, control-plane metrics');

  const url = new URL('/ws/events', base);
  url.protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
  const protocols = ['coldharbor', 'clearance.INNIE'];
  if (process.env.API_KEY) {
    const encodedKey = Buffer.from(process.env.API_KEY, 'utf8').toString('base64url');
    protocols.push(`api-key.${encodedKey}`);
  }
  socket = new WebSocket(url, protocols);
  socket.addEventListener('message', ({ data }) => {
    try { events.push(JSON.parse(data)); } catch { /* Ignore non-JSON keepalives. */ }
  });
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('WebSocket connect timeout')), 10000);
    socket.addEventListener('open', () => { clearTimeout(timer); resolve(); }, { once: true });
    socket.addEventListener('error', () => { clearTimeout(timer); reject(new Error('WebSocket connection failed')); }, { once: true });
  });
  const response = await request('/api/v1/compartments', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ context: 'INNIE', ownerId: 'verification-live', taskType: 'DATA_REDUCTION', payload: { batchSize: 500 } }),
  });
  assert.equal(response.status, 201);
  const job = await response.json();
  console.log(`Created ${job.compartmentId}`);
  const deadline = Date.now() + 30000;
  while (!events.some(e => e.compartmentId === job.compartmentId && e.toState === 'PURGED') && Date.now() < deadline) {
    await delay(200);
  }
  const jobEvents = events.filter(e => e.compartmentId === job.compartmentId);
  assert(jobEvents.some(e => e.toState === 'PURGED'), 'No terminal WebSocket event within 30s');
  const dd = await (await request(`/api/v1/compartments/${job.compartmentId}/deaddrop`)).json();
  assert.equal(dd.output.reducedSum, 49201);
  assert(dd.remainingTtlSeconds > 0, 'Expected live archive TTL');
  const canonical = JSON.stringify(dd.output, Object.keys(dd.output).sort());
  assert.equal(createHash('sha256').update(canonical).digest('hex'), dd.checksum);
  console.log(`PASS HTTP 201, ${jobEvents.length} job WebSocket events, PURGED, output, checksum, TTL`);
  console.log('Live smoke passed. This is not a browser interaction or crash-recovery test.');
} catch (error) {
  console.error(`FAIL: ${error.message}`);
  process.exitCode = 1;
} finally {
  socket?.close();
}
