export async function solvePow(
  salt: string,
  difficulty: number,
  _binding: { ipHash?: string; wsSession?: string } = {},
  timeoutMs = 60_000,
): Promise<string> {
  const worker = new Worker(new URL('./powWorker.ts', import.meta.url), { type: 'module' });
  return new Promise<string>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      worker.postMessage({ type: 'cancel' });
      worker.terminate();
      reject(new Error('PoW solve timeout'));
    }, timeoutMs);

    worker.onmessage = (event: MessageEvent<{ type: string; nonce?: string; message?: string }>) => {
      if (event.data.type === 'done' && event.data.nonce) {
        window.clearTimeout(timer);
        worker.terminate();
        resolve(event.data.nonce);
        return;
      }
      if (event.data.type === 'error') {
        window.clearTimeout(timer);
        worker.terminate();
        reject(new Error(event.data.message || 'PoW failed'));
      }
    };
    worker.onerror = (err) => {
      window.clearTimeout(timer);
      worker.terminate();
      reject(err.error || new Error('PoW worker failed'));
    };
    worker.postMessage({
      type: 'solve',
      salt,
      difficulty,
    });
  });
}
