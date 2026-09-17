import { afterEach, describe, it, expect, vi } from 'vitest';
import { canonicalizeJson, computeSha256, verifyChecksum } from '../utils/crypto';

describe('Crypto SHA-256 Utilities', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('should compute valid SHA-256 hash for empty string', async () => {
    const hash = await computeSha256('');
    expect(hash).toBe('e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855');
  });

  it('should compute consistent hash for payload objects', async () => {
    const payload = { processedCount: 500, status: 'SUCCESS' };
    const hash1 = await computeSha256(payload);
    const hash2 = await computeSha256(payload);
    expect(hash1).toBe(hash2);
    expect(hash1.length).toBe(64);
  });

  it('canonicalizes nested object keys like Go encoding/json', async () => {
    const first = { z: 1, nested: { y: 2, x: [{ b: true, a: false }] } };
    const second = { nested: { x: [{ a: false, b: true }], y: 2 }, z: 1 };
    expect(canonicalizeJson(first)).toBe('{"nested":{"x":[{"a":false,"b":true}],"y":2},"z":1}');
    expect(await computeSha256(first)).toBe(await computeSha256(second));
  });

  it('reports verification unavailable when SubtleCrypto is missing', async () => {
    vi.stubGlobal('crypto', {});
    await expect(computeSha256('payload')).rejects.toThrow(/SubtleCrypto.*not supported/i);
  });

  it('should correctly verify matching and mismatching checksums', () => {
    const valid = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    expect(verifyChecksum(valid, valid.toUpperCase())).toBe(true);
    expect(verifyChecksum(valid, '0000000000000000000000000000000000000000000000000000000000000000')).toBe(false);
    expect(verifyChecksum('', valid)).toBe(false);
  });
});
