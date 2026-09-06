import {
  RecordingPresets,
  requestRecordingPermissionsAsync,
  setAudioModeAsync,
  useAudioRecorder,
  useAudioRecorderState,
} from 'expo-audio';
import { useCallback, useEffect, useRef, useState } from 'react';

import type { PickedFile } from './pick';
import { shrink } from './waveform';

// Recording a voice note.
//
// The waveform under a voice note is not read back out of the file. Nobody
// does that: decoding every recording in a chat to draw fifty little bars
// would make scrolling cost more than listening. It is measured here, while
// the microphone is open and the numbers are free, and travels with the
// file. What the bubble draws was recorded, not computed.

// How long a note may run. A limit in bytes alone would allow an hour of
// speech, which is not a message.
export const MAX_MS = 10 * 60 * 1000;

// How often loudness is sampled. Ten a second is finer than the bars will
// ever be drawn, and cheap.
const SAMPLE_MS = 100;

// How many bars a note carries, matching what the hub accepts.
const BARS = 56;

// Decibels below which a signal is treated as silence. Speech into a phone
// sits around -30 dBFS; the floor is where the room noise is.
const FLOOR_DB = -55;

export interface RecorderHandle {
  recording: boolean;
  // How long it has been recording. Counted from the samples taken rather
  // than read from the recorder: every platform agrees on how many times
  // the microphone was measured, and they do not all agree on the clock.
  elapsedMs: number;
  // Loudness right now, 0 to 1, for the bar that is moving.
  level: number;
  // What has been measured so far, for the bars behind it.
  samples: number[];
  start: () => Promise<boolean>;
  // Ends the recording and returns it, ready to send.
  stop: () => Promise<PickedFile | null>;
  cancel: () => Promise<void>;
}

export function useRecorder(): RecorderHandle {
  const recorder = useAudioRecorder({ ...RecordingPresets.HIGH_QUALITY, isMeteringEnabled: true });
  const state = useAudioRecorderState(recorder, SAMPLE_MS);
  const [samples, setSamples] = useState<number[]>([]);
  const [recording, setRecording] = useState(false);
  // The samples are read at the end, off the render that collected them.
  const collected = useRef<number[]>([]);
  const level = state.metering === undefined ? 0 : levelOf(state.metering);

  useEffect(() => {
    if (!state.isRecording) return;
    collected.current = [...collected.current, level];
    setSamples(collected.current);
    // state.durationMillis moves every poll, which is what makes this the
    // sampler: one reading per tick of the recorder's own clock.
  }, [state.durationMillis, state.isRecording, level]);

  // A note that runs past the limit ends itself rather than being refused
  // after the fact, when the words are already spoken.
  useEffect(() => {
    if (state.isRecording && state.durationMillis >= MAX_MS) void recorder.stop();
  }, [recorder, state.durationMillis, state.isRecording]);

  const start = useCallback(async () => {
    const permission = await requestRecordingPermissionsAsync();
    if (!permission.granted) return false;
    // Recording and playback want different things from the device; asking
    // for the recording mode is what makes the microphone available.
    await setAudioModeAsync({ allowsRecording: true, playsInSilentMode: true });
    collected.current = [];
    setSamples([]);
    await recorder.prepareToRecordAsync();
    recorder.record();
    setRecording(true);
    return true;
  }, [recorder]);

  const finish = useCallback(async (): Promise<string | null> => {
    if (!recording) return null;
    setRecording(false);
    await recorder.stop();
    await setAudioModeAsync({ allowsRecording: false });
    return recorder.uri;
  }, [recorder, recording]);

  const stop = useCallback(async (): Promise<PickedFile | null> => {
    const durationMs = elapsed(state.durationMillis, collected.current.length);
    const uri = await finish();
    if (!uri) return null;
    return {
      uri,
      name: `voice-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.m4a`,
      mimeType: 'audio/mp4',
      kind: 'audio',
      byteSize: 0,
      durationMs,
      waveform: shrink(collected.current, BARS),
    };
  }, [finish, state.durationMillis]);

  const cancel = useCallback(async () => {
    await finish();
  }, [finish]);

  return {
    recording,
    elapsedMs: elapsed(state.durationMillis, samples.length),
    level,
    samples,
    start,
    stop,
    cancel,
  };
}

// elapsed is how long the recording has run: whichever of the recorder's
// own count and the sampler's is larger. A recorder that does not report a
// duration — some browsers do not — still produces a note of the right
// length, because the samples were taken on a clock of their own.
function elapsed(reported: number, samples: number): number {
  return Math.max(Math.round(reported), samples * SAMPLE_MS);
}

// levelOf turns decibels into a share of the way up a bar, which is what a
// waveform is: a shape, not a measurement.
function levelOf(db: number): number {
  if (!Number.isFinite(db)) return 0;
  const level = (db - FLOOR_DB) / -FLOOR_DB;
  return Math.min(1, Math.max(0, level));
}
