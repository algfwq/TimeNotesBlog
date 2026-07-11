/// <reference lib="webworker" />

function leadingZeroBits(bytes: Uint8Array): number {
  let count = 0;
  for (const b of bytes) {
    if (b === 0) {
      count += 8;
      continue;
    }
    for (let bit = 7; bit >= 0; bit -= 1) {
      if ((b & (1 << bit)) !== 0) {
        return count;
      }
      count += 1;
    }
  }
  return count;
}

type SolveMessage = {
  type: 'solve';
  salt: string;
  difficulty: number;
};

type CancelMessage = { type: 'cancel' };

let cancelled = false;
const encoder = new TextEncoder();

async function solve(msg: SolveMessage) {
  cancelled = false;
  const max = 50_000_000;
  for (let i = 0; i < max; i += 1) {
    if (cancelled) {
      (self as DedicatedWorkerGlobalScope).postMessage({ type: 'error', message: 'cancelled' });
      return;
    }
    const nonce = String(i);
    // Must match server: SHA-256(salt + nonce), leading zero bits.
    const digest = await crypto.subtle.digest('SHA-256', encoder.encode(msg.salt + nonce));
    if (leadingZeroBits(new Uint8Array(digest)) >= msg.difficulty) {
      (self as DedicatedWorkerGlobalScope).postMessage({ type: 'done', nonce });
      return;
    }
    if (i > 0 && i % 25000 === 0) {
      (self as DedicatedWorkerGlobalScope).postMessage({ type: 'progress', attempts: i });
      await new Promise((resolve) => setTimeout(resolve, 0));
    }
  }
  (self as DedicatedWorkerGlobalScope).postMessage({ type: 'error', message: 'PoW solve timeout' });
}

self.onmessage = (event: MessageEvent<SolveMessage | CancelMessage>) => {
  const data = event.data;
  if (data.type === 'cancel') {
    cancelled = true;
    return;
  }
  if (data.type === 'solve') {
    void solve(data);
  }
};
