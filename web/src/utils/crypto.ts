/**
 * Computes authentic SHA-256 hexadecimal string using browser Web Crypto API.
 */
function goJsonString(value: string): string {
  return JSON.stringify(value)
    .replace(/&/g, '\\u0026')
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029');
}

export function canonicalizeJson(value: unknown): string {
  if (value === null || typeof value !== 'object') {
    if (typeof value === 'string') return goJsonString(value);
    return JSON.stringify(value) ?? 'null';
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => canonicalizeJson(item ?? null)).join(',')}]`;
  }
  const object = value as Record<string, unknown>;
  const entries = Object.keys(object)
    .sort()
    .filter((key) => object[key] !== undefined)
    .map((key) => `${goJsonString(key)}:${canonicalizeJson(object[key])}`);
  return `{${entries.join(',')}}`;
}

export async function computeSha256(data: string | object): Promise<string> {
  const content = typeof data === 'string' ? data : canonicalizeJson(data);
  const encoder = new TextEncoder();
  const dataBuffer = encoder.encode(content);
  
  const subtle = globalThis.crypto?.subtle;
  if (!subtle) {
    throw new Error('Cryptographic verification unavailable: SubtleCrypto is not supported in this context');
  }

  const hashBuffer = await subtle.digest('SHA-256', dataBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

/**
 * Validates whether two checksums match (case-insensitive)
 */
export function verifyChecksum(claimed: string, computed: string): boolean {
  if (!claimed || !computed) return false;
  return claimed.trim().toLowerCase() === computed.trim().toLowerCase();
}
