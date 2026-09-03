import { describe, it, expect } from 'vitest';
import { computeSha256, verifyChecksum } from '../utils/crypto';

describe('Crypto SHA-256 Utilities', () => {
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

  it('should correctly verify matching and mismatching checksums', () => {
    const valid = 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855';
    expect(verifyChecksum(valid, valid.toUpperCase())).toBe(true);
    expect(verifyChecksum(valid, '0000000000000000000000000000000000000000000000000000000000000000')).toBe(false);
    expect(verifyChecksum('', valid)).toBe(false);
  });
});
