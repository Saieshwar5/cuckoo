import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import type { Conversation, DeliveryStatus, Message } from '@/api/types';
import { t } from '@/i18n';
import { sizes, spacing, type, useTheme, type Palette } from '@/theme';
import { plain } from '@/util/markdown';
import { formatListTime } from '@/util/time';

import { agentAvatar } from '@/media/avatar';

import { Avatar } from './Avatar';
import { WorkingDots } from './WorkingDots';

// counterpart is who a DM is with: the agent.
export function counterpart(c: Conversation) {
  return c.participants.find((p) => p.kind === 'agent') ?? c.participants[0];
}

// preview is the one line under the name. A stream in progress reads as
// typing; a tap reads as its label, which is its text.
export function preview(m: Message | null): string {
  if (!m) return t('chats.preview.none');
  if (m.status === 'streaming' && !m.body.text) return t('chats.preview.streaming');
  return plain(m.body.text ?? '');
}

// Ticks renders delivery status for our own last message the way every
// chat app does: one tick once the hub has it, two once the agent has it.
export function Ticks({ status, colors }: { status: DeliveryStatus; colors: Palette }) {
  switch (status) {
    case 'pending':
      return <Ionicons name="checkmark" size={16} color={colors.textSecondary} testID="ticks-pending" />;
    case 'delivered':
      return <Ionicons name="checkmark-done" size={16} color={colors.tickRead} testID="ticks-delivered" />;
    case 'failed':
      return <Ionicons name="alert-circle" size={16} color={colors.danger} testID="ticks-failed" />;
  }
}

// badge is how an unread count is written: exact up to 99, then "99+".
export function badge(n: number): string {
  return n > 99 ? '99+' : String(n);
}

export function ChatRow({
  conversation,
  onPress,
  pinned,
  muted,
  activity,
}: {
  conversation: Conversation;
  onPress: () => void;
  // What the person decided about this chat, drawn small at the right so
  // the row says it without saying it loudly.
  pinned?: boolean;
  muted?: boolean;
  // What the agent is doing right now, in words, in place of the last
  // message: the list says who is busy without opening anything.
  activity?: string | null;
}) {
  const { colors } = useTheme();
  const who = counterpart(conversation);
  const last = conversation.last_message;
  const mine = !activity && last?.sender.kind === 'user' && last.delivery_status;
  const unread = conversation.unread_count ?? 0;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [styles.row, pressed && { backgroundColor: colors.surface }]}
      testID={`chat-row-${conversation.id}`}
    >
      <Avatar
        name={who?.display_name ?? '?'}
        size={sizes.avatar}
        status={who?.kind === 'agent' ? (who.status ?? null) : undefined}
        source={who?.kind === 'agent' ? agentAvatar(who.id, who.has_avatar) : null}
      />
      <View style={styles.body}>
        <View style={styles.top}>
          <Text style={[styles.name, { color: colors.text }]} numberOfLines={1}>
            {who?.display_name ?? ''}
          </Text>
          {last ? (
            <Text
              style={[
                styles.time,
                { color: unread && !muted ? colors.accentStrong : colors.textSecondary },
                unread ? styles.timeUnread : null,
              ]}
            >
              {formatListTime(last.created_at, new Date(), t('chats.time.yesterday'))}
            </Text>
          ) : null}
        </View>
        <View style={styles.bottom}>
          {mine ? <Ticks status={mine} colors={colors} /> : null}
          {activity ? (
            <>
              <WorkingDots color={colors.accentStrong} />
              <Text
                style={[styles.preview, { color: colors.accentStrong }]}
                numberOfLines={1}
                testID={`row-activity-${conversation.id}`}
              >
                {activity}
              </Text>
            </>
          ) : (
            <Text
              style={[
                styles.preview,
                { color: last?.delivery_status === 'failed' ? colors.danger : colors.textSecondary },
              ]}
              numberOfLines={1}
            >
              {preview(last)}
            </Text>
          )}
          {muted ? (
            <Ionicons name="volume-mute-outline" size={15} color={colors.textSecondary} testID="row-muted" />
          ) : null}
          {pinned ? <Ionicons name="pin" size={14} color={colors.textSecondary} testID="row-pinned" /> : null}
          {unread ? (
            // A muted chat still counts, in grey: nothing interrupts, but
            // nothing is hidden either.
            <View
              style={[styles.badge, { backgroundColor: muted ? colors.textSecondary : colors.accent }]}
              accessibilityLabel={t('chats.unread', { count: unread })}
              testID={`row-unread-${conversation.id}`}
            >
              <Text style={[styles.badgeText, { color: colors.onAccent }]}>{badge(unread)}</Text>
            </View>
          ) : null}
        </View>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.lg,
    gap: spacing.md + 2,
  },
  body: { flex: 1, gap: 3 },
  top: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline', gap: spacing.sm },
  name: { ...type.headline, flexShrink: 1 },
  time: type.caption,
  timeUnread: { fontWeight: '600' },
  badge: {
    minWidth: 20,
    height: 20,
    borderRadius: 10,
    paddingHorizontal: 6,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badgeText: { fontSize: 12, fontWeight: '700' },
  bottom: { flexDirection: 'row', alignItems: 'center', gap: spacing.xs },
  preview: { ...type.secondary, flex: 1 },
});
