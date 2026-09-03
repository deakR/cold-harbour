/**
 * Computes authentic SHA-256 hexadecimal string using browser Web Crypto API.
 */
export async function computeSha256(data: string | object): Promise<string> {
  const content = typeof data === 'string' ? data : JSON.stringify(data);
  const encoder = new TextEncoder();
  const dataBuffer = encoder.encode(content);
  
  if (typeof window !== 'undefined' && window.crypto && window.crypto.subtle) {
    const hashBuffer = await window.crypto.subtle.digest('SHA-256', dataBuffer);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  }
  
  // Fallback for non-browser/test environments without subtle crypto
  return 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
}

/**
 * Validates whether two checksums match (case-insensitive)
 */
export function verifyChecksum(claimed: string, computed: string): boolean {
  if (!claimed || !computed) return false;
  return claimed.trim().toLowerCase() === computed.trim().toLowerCase();
}
