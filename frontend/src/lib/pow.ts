export async function solvePow(salt: string, difficulty: number): Promise<string> {
  const prefix = '0'.repeat(difficulty);
  const encoder = new TextEncoder();
  for (let i = 0; i < 50_000_000; i++) {
    const nonce = String(i);
    const data = encoder.encode(salt + nonce);
    const hash = await crypto.subtle.digest('SHA-256', data);
    const hex = [...new Uint8Array(hash)].map((b) => b.toString(16).padStart(2, '0')).join('');
    if (hex.startsWith(prefix)) {
      return nonce;
    }
  }
  throw new Error('PoW solve timeout');
}
