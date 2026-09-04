import {useState} from 'react';

const App = window.go.main.App;

export function parseValues(text: string): number[] {
    const parts = text.split(/[\s,;\n]+/).map((s) => s.trim()).filter(Boolean);
    const nums = parts.map(Number);
    const bad = nums.findIndex((n) => !Number.isFinite(n));
    if (bad >= 0) throw new Error(`"${parts[bad]}" is not a number`);
    if (nums.length === 0) throw new Error('enter at least one number');
    return nums;
}

export function shortChecksum(sum: string): string {
    return sum ? `${sum.slice(0, 12)}…` : '—';
}

export function useAsync<T>(fn: () => Promise<T>) {
    const [data, setData] = useState<T | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const run = async () => {
        setLoading(true);
        setError(null);
        try {
            setData(await fn());
        } catch (e: any) {
            setError(String(e?.message ?? e));
        } finally {
            setLoading(false);
        }
    };
    return {data, error, loading, run, setData};
}

export {App as Backend};
