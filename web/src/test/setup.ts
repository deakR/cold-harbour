import '@testing-library/jest-dom';

// Polyfill window.crypto if needed in test environment
if (typeof window !== 'undefined' && (!window.crypto || !window.crypto.subtle)) {
  const nodeCrypto = require('crypto');
  // @ts-ignore
  (window as any).crypto = {
    subtle: {
      digest: async (_algorithm: any, data: any): Promise<any> => {
        const hash = nodeCrypto.createHash('sha256');
        const buf = Buffer.isBuffer(data) ? data : Buffer.from(data as any);
        hash.update(buf);
        return hash.digest();
      },
    } as any,
    getRandomValues: (arr: any) => nodeCrypto.randomFillSync(arr),
  };
}

