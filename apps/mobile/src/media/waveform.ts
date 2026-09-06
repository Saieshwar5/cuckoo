// The shape of a recording, as numbers.
//
// Pure arithmetic, deliberately apart from the recorder and the player:
// this is the part with decisions in it, and it can be tested without a
// microphone or a native module anywhere near it.

// shrink reduces however many loudness samples were taken to a fixed
// number of bars, by averaging each group, and scales them to 0 to 100.
// A ten-second note and a two-minute one then look like the same kind of
// thing rather than one being a wall.
export function shrink(samples: number[], bars: number): number[] {
  if (samples.length === 0) return [];
  const out: number[] = [];
  for (let i = 0; i < bars; i++) {
    const from = Math.floor((i * samples.length) / bars);
    const to = Math.max(from + 1, Math.floor(((i + 1) * samples.length) / bars));
    let sum = 0;
    for (let j = from; j < to; j++) sum += samples[j] ?? 0;
    out.push(Math.round((sum / (to - from)) * 100));
  }
  return out;
}

// bars fits what a recording carried to what a bubble draws: a short note
// keeps its own shape, a longer one is thinned rather than stretched, and
// one that carried nothing gets a flat line to seek along.
export function bars(waveform: number[] | undefined, count: number, flat = 18): number[] {
  if (!waveform?.length) return new Array(count).fill(flat);
  if (waveform.length <= count) return waveform;
  const out: number[] = [];
  for (let i = 0; i < count; i++) {
    out.push(waveform[Math.floor((i * waveform.length) / count)] ?? 0);
  }
  return out;
}
