import { useCallback, useEffect, useState } from 'react';

// useCountdown counts a number of seconds down to zero and can be restarted.
// It ticks once a second while running and not at all once done.
export function useCountdown(seconds: number): { remaining: number; restart: () => void } {
  const [deadline, setDeadline] = useState(() => Date.now() + seconds * 1000);
  const [now, setNow] = useState(() => Date.now());
  const done = now >= deadline;

  useEffect(() => {
    if (done) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [deadline, done]);

  const restart = useCallback(() => {
    const at = Date.now();
    setDeadline(at + seconds * 1000);
    setNow(at);
  }, [seconds]);

  return { remaining: done ? 0 : Math.ceil((deadline - now) / 1000), restart };
}
