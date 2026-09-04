import {useCallback, useEffect, useRef, useState} from 'react';
import './batchseal.css';
import {Backend, parseValues, shortChecksum} from './api';

type View = 'batch' | 'live' | 'receipts' | 'fleet' | 'settings';

interface Tracked {
    id: string;
    taskType: string;
    state: string;
    progress: number;
    receipt?: BSReceipt;
    error?: string;
}

const TERMINAL = new Set(['PURGED', 'ARCHIVED', 'COMPLETED', 'FAILED']);

function Main() {
    const [view, setView] = useState<View>('batch');
    const [cfg, setCfg] = useState<BSConfig | null>(null);
    const [taskType, setTaskType] = useState('DATA_REDUCTION');
    const [valuesText, setValuesText] = useState('1200, 3400, 560, 890, 1250');
    const [formError, setFormError] = useState<string | null>(null);
    const [tracked, setTracked] = useState<Tracked[]>([]);
    const [receipts, setReceipts] = useState<BSReceipt[]>([]);
    const [audits, setAudits] = useState<BSAudit[]>([]);
    const [workers, setWorkers] = useState<BSWorker[]>([]);
    const [dlqSize, setDlqSize] = useState(0);
    const timers = useRef<Record<string, number>>({});

    useEffect(() => {
        Backend.GetConfig().then(setCfg).catch(() => setCfg({baseURL: 'http://localhost:8080', apiKey: '', clearance: 'INNIE', ownerID: 'usr_batchseal'}));
    }, []);

    const stopPoll = useCallback((id: string) => {
        const t = timers.current[id];
        if (t) {
            window.clearInterval(t);
            delete timers.current[id];
        }
    }, []);

    const poll = useCallback((id: string) => {
        const tick = async () => {
            try {
                const st = await Backend.BatchStatus(id);
                let patch: Partial<Tracked> = {state: st.state, progress: st.progress, error: undefined};
                if (TERMINAL.has(st.state)) {
                    stopPoll(id);
                    try {
                        patch.receipt = await Backend.BatchReceipt(id);
                    } catch (e: any) {
                        patch.error = st.state === 'FAILED' ? 'Batch failed — see dead letters for redrive.' : String(e?.message ?? e);
                    }
                }
                setTracked((prev) => prev.map((b) => (b.id === id ? {...b, ...patch} : b)));
            } catch (e: any) {
                setTracked((prev) => prev.map((b) => (b.id === id ? {...b, error: String(e?.message ?? e)} : b)));
            }
        };
        void tick();
        timers.current[id] = window.setInterval(tick, 2000);
    }, [stopPoll]);

    useEffect(() => () => Object.keys(timers.current).forEach(stopPoll), [stopPoll]);

    const dispatch = async (chaos: boolean) => {
        setFormError(null);
        let values: number[];
        try {
            values = parseValues(valuesText);
        } catch (e: any) {
            setFormError(e.message);
            return;
        }
        try {
            const id = chaos ? await Backend.DispatchChaos(taskType, values) : await Backend.DispatchBatch(taskType, values);
            setTracked((prev) => [{id, taskType, state: 'QUEUED', progress: 0}, ...prev]);
            poll(id);
            setView('live');
        } catch (e: any) {
            setFormError(String(e?.message ?? e));
        }
    };

    const refreshAll = useCallback(async () => {
        try {
            const [rc, au, fl, dlq] = await Promise.all([
                Backend.LocalReceipts(), Backend.AuditHistory(''), Backend.Fleet(), Backend.DeadLetters(),
            ]);
            setReceipts(rc);
            setAudits(au.slice(0, 50));
            setWorkers(fl);
            setDlqSize(dlq);
        } catch {
            /* offline: keep last known */
        }
    }, []);

    useEffect(() => {
        void refreshAll();
        const t = window.setInterval(refreshAll, 8000);
        return () => window.clearInterval(t);
    }, [refreshAll]);

    const redrive = async () => {
        try {
            await Backend.RedriveDLQ(10);
            await refreshAll();
        } catch (e: any) {
            setFormError(String(e?.message ?? e));
        }
    };

    const saveSettings = async () => {
        if (!cfg) return;
        try {
            await Backend.SaveConfig(cfg);
            setFormError(null);
        } catch (e: any) {
            setFormError(String(e?.message ?? e));
        }
    };

    return (
        <div className="bs-root">
            <header className="bs-header">
                <div className="bs-brand">
                    <span className="bs-seal">◈</span>
                    <div>
                        <h1>BatchSeal</h1>
                        <p>Verifiable batch computation on ColdHarbor</p>
                    </div>
                </div>
                <nav className="bs-nav">
                    {(['batch', 'live', 'receipts', 'fleet', 'settings'] as View[]).map((v) => (
                        <button key={v} className={view === v ? 'active' : ''} onClick={() => setView(v)}>
                            {v === 'batch' ? 'New batch' : v === 'live' ? `Live (${tracked.filter((b) => !TERMINAL.has(b.state)).length})` : v === 'receipts' ? 'Receipts' : v === 'fleet' ? 'Fleet' : 'Settings'}
                        </button>
                    ))}
                </nav>
            </header>

            {formError && <div className="bs-error">{formError}</div>}

            {view === 'batch' && (
                <section className="bs-card">
                    <h2>New verifiable batch</h2>
                    <p className="bs-hint">Each batch runs in an isolated ColdHarbor compartment with checkpointing. The sealed result carries a SHA-256 receipt verified locally before it is stored.</p>
                    <label>Task type</label>
                    <select value={taskType} onChange={(e) => setTaskType(e.target.value)}>
                        <option value="DATA_REDUCTION">DATA_REDUCTION — totals and sums</option>
                        <option value="CIPHER_STREAM">CIPHER_STREAM — hash-chained digest</option>
                        <option value="ARCHIVE_SEAL">ARCHIVE_SEAL — sealed count with checksum</option>
                    </select>
                    <label>Values (comma, space, or line separated)</label>
                    <textarea rows={5} value={valuesText} onChange={(e) => setValuesText(e.target.value)} />
                    <div className="bs-row">
                        <button className="bs-primary" onClick={() => dispatch(false)}>Seal batch</button>
                        <button className="bs-danger-ghost" onClick={() => dispatch(true)}>Seal with crash probe</button>
                    </div>
                </section>
            )}

            {view === 'live' && (
                <section className="bs-card">
                    <h2>Live batches</h2>
                    {tracked.length === 0 && <p className="bs-hint">No batches yet. Dispatch one to watch checkpoints arrive.</p>}
                    {tracked.map((b) => (
                        <div key={b.id} className="bs-batch">
                            <div className="bs-batch-head">
                                <code>{b.id}</code>
                                <span className={`bs-state bs-${b.state.toLowerCase()}`}>{b.state} {b.progress}%</span>
                            </div>
                            <div className="bs-bar"><div style={{width: `${b.progress}%`}} /></div>
                            {b.receipt && (
                                <div className="bs-receipt-line">
                                    <span className={b.receipt.verified ? 'bs-ok' : 'bs-bad'}>
                                        {b.receipt.verified ? '✓ sealed & verified' : '✗ seal mismatch'}
                                    </span>
                                    <code>{shortChecksum(b.receipt.checksum)}</code>
                                    <span className="bs-hint">TTL {b.receipt.remainingTtlSeconds}s</span>
                                </div>
                            )}
                            {b.error && <div className="bs-error">{b.error}</div>}
                        </div>
                    ))}
                </section>
            )}

            {view === 'receipts' && (
                <section className="bs-card">
                    <h2>Receipts & audit</h2>
                    <h3>Local sealed copies ({receipts.length})</h3>
                    {receipts.map((r) => (
                        <div key={r.compartmentId} className="bs-row-line">
                            <code>{r.compartmentId}</code>
                            <span className={r.verified ? 'bs-ok' : 'bs-bad'}>{r.verified ? '✓' : '✗'}</span>
                            <code>{shortChecksum(r.checksum)}</code>
                        </div>
                    ))}
                    <h3>Durable audit (latest {audits.length})</h3>
                    {audits.map((a) => (
                        <div key={a.id} className="bs-row-line">
                            <code>{a.compartmentId}</code>
                            <span className={`bs-state bs-${a.finalState.toLowerCase()}`}>{a.finalState}</span>
                            <code>{shortChecksum(a.checksum)}</code>
                        </div>
                    ))}
                </section>
            )}

            {view === 'fleet' && (
                <section className="bs-card">
                    <div className="bs-row-between">
                        <h2>Fleet & dead letters</h2>
                        <div className="bs-row">
                            <span className={dlqSize > 0 ? 'bs-bad' : 'bs-ok'}>DLQ depth: {dlqSize}</span>
                            {dlqSize > 0 && <button className="bs-primary" onClick={redrive}>Redrive 10</button>}
                        </div>
                    </div>
                    {workers.map((w) => (
                        <div key={w.workerId} className="bs-row-line">
                            <code>{w.workerId}</code>
                            <span className={w.healthy ? 'bs-ok' : 'bs-bad'}>{w.status}</span>
                            <span className="bs-hint">{w.activeCompartmentId || 'idle'}</span>
                        </div>
                    ))}
                    {workers.length === 0 && <p className="bs-hint">No workers reporting. Start the Go worker engine.</p>}
                </section>
            )}

            {view === 'settings' && cfg && (
                <section className="bs-card bs-settings">
                    <h2>Connection</h2>
                    <label>Control plane base URL</label>
                    <input value={cfg.baseURL} onChange={(e) => setCfg({...cfg, baseURL: e.target.value})} />
                    <label>API key (optional)</label>
                    <input type="password" value={cfg.apiKey} onChange={(e) => setCfg({...cfg, apiKey: e.target.value})} />
                    <label>Clearance</label>
                    <select value={cfg.clearance} onChange={(e) => setCfg({...cfg, clearance: e.target.value})}>
                        {['INNIE', 'OUTIE', 'SYSTEM', 'ADMIN'].map((c) => <option key={c} value={c}>{c}</option>)}
                    </select>
                    <label>Owner ID</label>
                    <input value={cfg.ownerID} onChange={(e) => setCfg({...cfg, ownerID: e.target.value})} />
                    <div className="bs-row"><button className="bs-primary" onClick={saveSettings}>Save</button></div>
                </section>
            )}

            <footer className="bs-footer">Memory is temporary. Results are permanent. Context must never leak.</footer>
        </div>
    );
}

export default function App() {
    return <Main />;
}
