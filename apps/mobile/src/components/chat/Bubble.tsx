import { Ionicons } from '@expo/vector-icons';
import React, { useEffect, useState } from 'react';
import { Animated, Pressable, StyleSheet, Text, View } from 'react-native';

import type { Button as ButtonSpec, DeliveryStatus } from '@/api/types';
import type { ChatMessage } from '@/chat/store';
import { t } from '@/i18n';
import { radius, spacing, type, useTheme, type Palette } from '@/theme';
import { plain } from '@/util/markdown';
import { formatClock } from '@/util/time';

import { Attachments } from './Attachments';
import { RichText } from './RichText';

interface Props {
  message: ChatMessage;
  mine: boolean;
  // The first bubble of a run from one sender carries the tail.
  first: boolean;
  agentName: string;
  onReply: (m: ChatMessage) => void;
  // A choice was made: an id button was tapped. A url button never comes
  // through here, because it is not a choice.
  onButton: (m: ChatMessage, b: ButtonSpec) => void;
  // The person wants to leave: a link in the words, or a url button.
  onOpen: (url: string) => void;
  onRetry: (key: string) => void;
}

// Bubble is one message: what was said, what it quoted, the buttons it
// offered, and when. Ours sit right in the accent's shade; the agent's sit
// left on a surface.
export function Bubble({ message: m, mine, first, agentName, onReply, onButton, onOpen, onRetry }: Props) {
  const { colors } = useTheme();
  const failed = m.delivery_status === 'failed' && m.localKey;
  const meta = mine ? colors.bubbleMetaMine : colors.bubbleMetaTheirs;
  const ink = mine ? colors.bubbleTextMine : colors.bubbleTextTheirs;
  const quoteBar = mine ? colors.quoteBarMine : colors.quoteBarTheirs;
  return (
    <View style={[styles.row, mine ? styles.rowMine : styles.rowTheirs]}>
      <Pressable
        onLongPress={() => onReply(m)}
        onPress={failed ? () => onRetry(m.localKey as string) : undefined}
        delayLongPress={300}
        testID={`bubble-${m.id}`}
        style={[
          styles.bubble,
          { backgroundColor: mine ? colors.bubbleMine : colors.bubbleTheirs },
          !mine && { borderWidth: StyleSheet.hairlineWidth, borderColor: colors.bubbleBorder },
          first && (mine ? styles.tailMine : styles.tailTheirs),
        ]}
      >
        {m.reply_to ? (
          <View
            style={[
              styles.quote,
              { backgroundColor: mine ? colors.bubbleQuoteMine : colors.bubbleQuoteTheirs },
            ]}
          >
            <View style={[styles.quoteBar, { backgroundColor: quoteBar }]} />
            <View style={styles.quoteBody}>
              <Text style={[styles.quoteName, { color: quoteBar }]} numberOfLines={1}>
                {m.reply_to.sender_kind === 'user' ? t('chat.reply.you') : agentName}
              </Text>
              <Text style={[styles.quoteText, { color: meta }]} numberOfLines={2}>
                {plain(m.reply_to.text_preview)}
              </Text>
            </View>
          </View>
        ) : null}
        {m.body.attachments?.length ? (
          <Attachments attachments={m.body.attachments} mine={mine} sending={!!m.localKey} />
        ) : null}
        {m.body.text || m.status === 'streaming' ? (
          <RichText
            text={m.body.text ?? ''}
            style={[styles.text, { color: ink }]}
            trailing={m.status === 'streaming' ? <Caret color={ink} /> : null}
            onLink={onOpen}
          />
        ) : null}
        <View style={styles.footer}>
          {m.truncated ? (
            <Text style={[styles.meta, styles.truncated, { color: meta }]}>{t('chat.truncated')}</Text>
          ) : null}
          <Text style={[styles.meta, { color: meta }]}>{formatClock(m.created_at)}</Text>
          {mine && m.delivery_status ? <Ticks status={m.delivery_status} colors={colors} /> : null}
        </View>
        {m.body.buttons?.length ? (
          <Buttons
            rows={m.body.buttons}
            selected={m.body.selected_button_id}
            colors={colors}
            onPress={(b) => (b.url ? onOpen(b.url) : onButton(m, b))}
          />
        ) : null}
      </Pressable>
      {failed ? (
        <Text style={[styles.failed, { color: colors.danger }]} testID={`failed-${m.id}`}>
          {t('chat.failed')}
        </Text>
      ) : null}
    </View>
  );
}

// Ticks are delivery to the agent's backend: one once the hub has the
// message, two once the backend does.
function Ticks({ status, colors }: { status: DeliveryStatus; colors: Palette }) {
  switch (status) {
    case 'pending':
      return <Ionicons name="checkmark" size={15} color={colors.bubbleMetaMine} testID="tick-pending" />;
    case 'delivered':
      return (
        <Ionicons name="checkmark-done" size={15} color={colors.bubbleTextMine} testID="tick-delivered" />
      );
    case 'failed':
      return <Ionicons name="alert-circle" size={15} color={colors.danger} testID="tick-failed" />;
  }
}

// Buttons are the agent's question as things to tap. Once one is taken the
// choice stays visible and the rest step back — except a link, which is not
// an answer to anything and still works afterwards.
function Buttons({
  rows,
  selected,
  colors,
  onPress,
}: {
  rows: ButtonSpec[][];
  selected: string | undefined;
  colors: Palette;
  onPress: (b: ButtonSpec) => void;
}) {
  const done = selected !== undefined && selected !== '';
  return (
    <View style={[styles.buttons, { borderTopColor: colors.hairline }]}>
      {rows.map((row, i) => (
        <View key={i} style={styles.buttonRow}>
          {row.map((b, j) => {
            const link = !!b.url;
            const chosen = !link && !!b.id && b.id === selected;
            const spent = done && !link;
            const primary = b.style === 'primary' && !spent;
            const fg = primary ? colors.onAccent : b.style === 'danger' ? colors.danger : colors.accent;
            return (
              <Pressable
                key={b.id ?? b.url ?? j}
                accessibilityRole={link ? 'link' : 'button'}
                accessibilityState={{ disabled: spent, selected: chosen }}
                accessibilityHint={link ? t('chat.button.leaves') : undefined}
                disabled={spent}
                onPress={() => onPress(b)}
                testID={link ? `open-${b.url}` : `button-${b.id}`}
                style={[
                  styles.button,
                  {
                    backgroundColor: primary
                      ? colors.accent
                      : chosen
                        ? colors.accentTint
                        : colors.surfaceStrong,
                  },
                  spent && !chosen && styles.dimmed,
                ]}
              >
                {chosen ? <Ionicons name="checkmark" size={16} color={fg} /> : null}
                <Text style={[styles.buttonLabel, { color: fg }]} numberOfLines={1}>
                  {b.label}
                </Text>
                {link ? <Ionicons name="open-outline" size={14} color={fg} /> : null}
              </Pressable>
            );
          })}
        </View>
      ))}
    </View>
  );
}

// Caret blinks at the end of a reply still being written.
function Caret({ color }: { color: string }) {
  const [opacity] = useState(() => new Animated.Value(1));
  useEffect(() => {
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(opacity, { toValue: 0, duration: 450, useNativeDriver: false }),
        Animated.timing(opacity, { toValue: 1, duration: 450, useNativeDriver: false }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [opacity]);
  return <Animated.Text style={{ color, opacity }}>{' ▍'}</Animated.Text>;
}

const styles = StyleSheet.create({
  row: { marginVertical: 1.5, paddingHorizontal: spacing.sm },
  rowMine: { alignItems: 'flex-end' },
  rowTheirs: { alignItems: 'flex-start' },
  bubble: {
    maxWidth: '84%',
    minWidth: 88,
    borderRadius: radius.md,
    paddingHorizontal: spacing.sm + 2,
    paddingTop: spacing.sm - 2,
    paddingBottom: spacing.xs + 1,
  },
  tailMine: { borderTopRightRadius: 4 },
  tailTheirs: { borderTopLeftRadius: 4 },
  quote: {
    flexDirection: 'row',
    borderRadius: radius.sm,
    overflow: 'hidden',
    marginBottom: spacing.xs + 2,
    marginTop: 2,
  },
  quoteBar: { width: 4 },
  quoteBody: { flex: 1, paddingHorizontal: spacing.sm, paddingVertical: spacing.xs + 1 },
  quoteName: { ...type.label, marginBottom: 1 },
  quoteText: type.secondary,
  text: { ...type.body, lineHeight: 22 },
  footer: { flexDirection: 'row', alignItems: 'center', justifyContent: 'flex-end', gap: 3, marginTop: 1 },
  meta: { fontSize: 11 },
  truncated: { fontStyle: 'italic', marginRight: spacing.xs },
  buttons: {
    borderTopWidth: StyleSheet.hairlineWidth,
    marginTop: spacing.sm,
    paddingTop: spacing.xs,
    gap: spacing.xs + 2,
  },
  buttonRow: { flexDirection: 'row', gap: spacing.xs + 2 },
  button: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: spacing.xs,
    minHeight: 38,
    paddingHorizontal: spacing.sm,
    borderRadius: radius.sm,
  },
  buttonLabel: { ...type.secondary, fontWeight: '600' },
  dimmed: { opacity: 0.45 },
  failed: { ...type.caption, marginTop: 2, marginRight: spacing.xs },
});
