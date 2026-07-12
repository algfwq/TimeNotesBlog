export type PowProgress = {
  attempts: number;
  difficulty: number;
  /** 0–99 while solving; 100 when done */
  percent: number;
  status: 'solving' | 'done' | 'error';
};

export type PowOptions = {
  binding?: { ipHash?: string; wsSession?: string };
  timeoutMs?: number;
  onProgress?: (p: PowProgress) => void;
};

/** Estimate progress from attempts vs expected work ~ 2^difficulty. */
function progressPercent(attempts: number, difficulty: number): number {
  const expected = Math.pow(2, Math.max(1, Math.min(difficulty, 28)));
  // asymptotic curve so easy challenges fill quickly without jumping to 100 early
  const ratio = Math.min(1, attempts / Math.max(expected * 0.85, 1));
  return Math.min(99, Math.round(ratio * 99));
}

export async function solvePow(
  salt: string,
  difficulty: number,
  options: PowOptions | { ipHash?: string; wsSession?: string } = {},
  timeoutMsArg?: number,
): Promise<string> {
  // Backward compatible: third arg may be binding object used previously.
  const opts: PowOptions = typeof (options as PowOptions).onProgress === 'function' || 'timeoutMs' in (options as PowOptions) || 'binding' in (options as PowOptions)
    ? (options as PowOptions)
    : { binding: options as { ipHash?: string; wsSession?: string } };
  const timeoutMs = opts.timeoutMs ?? timeoutMsArg ?? 60_000;
  const onProgress = opts.onProgress;

  const worker = new Worker(new URL('./powWorker.ts', import.meta.url), { type: 'module' });
  return new Promise<string>((resolve, reject) => {
    const timer = window.setTimeout(() => {
      worker.postMessage({ type: 'cancel' });
      worker.terminate();
      onProgress?.({ attempts: 0, difficulty, percent: 0, status: 'error' });
      reject(new Error('PoW solve timeout'));
    }, timeoutMs);

    onProgress?.({ attempts: 0, difficulty, percent: 1, status: 'solving' });

    worker.onmessage = (event: MessageEvent<{ type: string; nonce?: string; message?: string; attempts?: number }>) => {
      if (event.data.type === 'progress') {
        const attempts = Number(event.data.attempts || 0);
        onProgress?.({
          attempts,
          difficulty,
          percent: progressPercent(attempts, difficulty),
          status: 'solving',
        });
        return;
      }
      if (event.data.type === 'done' && event.data.nonce) {
        window.clearTimeout(timer);
        worker.terminate();
        onProgress?.({ attempts: Number(event.data.attempts || 0), difficulty, percent: 100, status: 'done' });
        resolve(event.data.nonce);
        return;
      }
      if (event.data.type === 'error') {
        window.clearTimeout(timer);
        worker.terminate();
        onProgress?.({ attempts: 0, difficulty, percent: 0, status: 'error' });
        reject(new Error(event.data.message || 'PoW failed'));
      }
    };
    worker.onerror = (err) => {
      window.clearTimeout(timer);
      worker.terminate();
      onProgress?.({ attempts: 0, difficulty, percent: 0, status: 'error' });
      reject(err.error || new Error('PoW worker failed'));
    };
    worker.postMessage({
      type: 'solve',
      salt,
      difficulty,
    });
  });
}
