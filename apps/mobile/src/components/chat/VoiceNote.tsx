import { Ionicons } from '@expo/vector-icons';
import { useAudioPlayer, useAudioPlayerStatus } from 'expo-audio';
import React, { useMemo, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import type { Attachment } from '@/api/types';
import { t } from '@/i18n';
import { useMediaSource } from '@/media/source';
import { bars as fit } from '@/media/waveform';
import { radius, spacing, type, type Palette } from '@/theme';
import { formatDuration } from '@/util/time';

// A voice note: play, a waveform that fills as it goes, how long it is,
// and how fast it is being played.
//
// The bars are the numbers the recording carried, drawn straight. No audio
// is decoded to draw them, which is why a chat full of voice notes scrolls
// like a chat full of words.

// How many bars fit across a bubble. A note with fewer is drawn with
// fewer; one with more is thinned, so every note is the same width.
const BARS = 40;
const BAR_WIDTH = 3;
const BAR_GAP = 2;
const BAR_MAX = 26;
const BAR_MIN = 3;

// The speeds worth having. Nobody wants 1.25 on a voice note; people want
// "normal" and "get on with it".
const SPEEDS = [1, 1.5, 2] as const;

interface Props {
  attachment: Attachment;
  mine: boolean;
  sending: boolean;
  colors: Palette;
}

export function VoiceNote({ attachment, mine, sending, colors }: Props) {
  const { source, gone } = useMediaSource(attachment.media_id, { localUri: attachment.local_uri });
  const player = useAudioPlayer(source ?? null);
  const status = useAudioPlayerStatus(player);
  const [speed, setSpeed] = useState(0);

  const ink = mine ? colors.bubbleTextMine : colors.bubbleTextTheirs;
  const meta = mine ? colors.bubbleMetaMine : colors.bubbleMetaTheirs;
  const spent = mine ? colors.bubbleMetaMine : colors.textSecondary;

  const bars = useMemo(() => fit(attachment.waveform, BARS), [attachment.waveform]);
  // The hub was told how long it is; the player only knows once it has
  // loaded. Before then the recording's own number is what is shown.
  const total = status.duration || (attachment.duration_ms ?? 0) / 1000;
  const played = status.currentTime;
  const progress = total > 0 ? Math.min(1, played / total) : 0;
  // The bars are the recording's own shape and travel with the message,
  // so a note the hub has swept still shows what it looked like; only
  // the sound is gone, and the bars stay dim to say so.
  const reached = (i: number) => !gone && i / bars.length <= progress;

  const toggle = () => {
    if (status.playing) {
      player.pause();
      return;
    }
    // A note that has run to the end starts again rather than doing
    // nothing, which is what a second tap on it means.
    if (total > 0 && played >= total - 0.05) void player.seekTo(0);
    player.play();
  };

  const cycleSpeed = () => {
    const next = (speed + 1) % SPEEDS.length;
    setSpeed(next);
    player.setPlaybackRate(SPEEDS[next] as number);
  };

  return (
    <View style={styles.row} testID={`voice-${attachment.media_id}`}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={status.playing ? t('chat.voice.pause') : t('chat.voice.play')}
        accessibilityState={{ disabled: sending || gone }}
        disabled={sending || !source}
        onPress={toggle}
        hitSlop={8}
        testID={`voice-play-${attachment.media_id}`}
        style={[styles.play, { backgroundColor: mine ? colors.bubbleQuoteMine : colors.surfaceStrong }]}
      >
        <Ionicons
          name={status.playing ? 'pause' : 'play'}
          size={18}
          color={ink}
          style={status.playing ? undefined : styles.nudge}
        />
      </Pressable>

      <View style={styles.body}>
        <Pressable
          accessibilityRole="adjustable"
          accessibilityLabel={t('chat.voice.seek')}
          disabled={sending || gone || total === 0}
          onPress={(e) => {
            const width = BARS * (BAR_WIDTH + BAR_GAP);
            void player.seekTo(Math.max(0, Math.min(1, e.nativeEvent.locationX / width)) * total);
          }}
          style={styles.bars}
        >
          {bars.map((value, i) => (
            <View
              key={i}
              style={[
                styles.bar,
                {
                  height: BAR_MIN + (BAR_MAX - BAR_MIN) * (value / 100),
                  backgroundColor: reached(i) ? ink : spent,
                  opacity: reached(i) ? 1 : 0.4,
                },
              ]}
            />
          ))}
        </Pressable>
        <View style={styles.footer}>
          {gone ? (
            <Text style={[styles.gone, { color: meta }]} testID={`gone-${attachment.media_id}`}>
              {t('chat.media.gone')}
            </Text>
          ) : (
            <>
              <Text style={[styles.time, { color: meta }]}>
                {formatDuration(status.playing || played > 0 ? played : total)}
              </Text>
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t('chat.voice.speed')}
                onPress={cycleSpeed}
                hitSlop={8}
                testID={`voice-speed-${attachment.media_id}`}
              >
                <Text style={[styles.speed, { color: meta, borderColor: meta }]}>{SPEEDS[speed]}×</Text>
              </Pressable>
            </>
          )}
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingVertical: 2 },
  play: { width: 38, height: 38, borderRadius: radius.pill, alignItems: 'center', justifyContent: 'center' },
  nudge: { marginLeft: 2 },
  body: { gap: 2 },
  bars: { flexDirection: 'row', alignItems: 'center', gap: BAR_GAP, height: BAR_MAX },
  bar: { width: BAR_WIDTH, borderRadius: 1.5 },
  footer: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  time: { ...type.caption, fontVariant: ['tabular-nums'] },
  gone: type.caption,
  speed: {
    ...type.caption,
    fontWeight: '600',
    borderWidth: StyleSheet.hairlineWidth,
    borderRadius: radius.sm,
    paddingHorizontal: 5,
    paddingVertical: 1,
    overflow: 'hidden',
  },
});
