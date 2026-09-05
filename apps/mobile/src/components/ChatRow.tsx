import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import type { Conversation, Message } from '@/api/types';
import { t } from '@/i18n';
import { colors, spacing, type } from '@/theme/tokens';
import { formatListTime } from '@/util/time';

import { Avatar } from './Avatar';

// counterpart is who a DM is with: the agent.
export function counterpart(c: Conversation) {
  return c.participants.find((p) => p.kind === 'agent') ?? c.participants[0];
}

// preview is the one line under the name. A stream in progress reads as
// typing; a tap reads as its label, which is its text.
export function preview(m: Message | null): string {
  if (!m) return t('chats.preview.none');
  if (m.status === 'streaming' && !m.body.text) return t('chats.preview.streaming');
  return m.body.text ?? '';
}

// ticks renders delivery status for our own last message, WhatsApp-style.
function ticks(m: Message | null): string {
  if (!m || m.sender.kind !== 'user' || !m.delivery_status) return '';
  switch (m.delivery_status) {
    case 'pending':
      return '✓ ';
    case 'delivered':
      return '✓✓ ';
    case 'failed':
      return '! ';
  }
}

export function ChatRow({ conversation, onPress }: { conversation: Conversation; onPress: () => void }) {
  const who = counterpart(conversation);
  const last = conversation.last_message;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [styles.row, pressed && styles.pressed]}
      testID={`chat-row-${conversation.id}`}
    >
      <Avatar
        name={who?.display_name ?? '?'}
        status={who?.kind === 'agent' ? (who.status ?? null) : undefined}
      />
      <View style={styles.body}>
        <View style={styles.top}>
          <Text style={styles.name} numberOfLines={1}>
            {who?.display_name ?? ''}
          </Text>
          {last ? (
            <Text style={styles.time}>
              {formatListTime(last.created_at, new Date(), t('chats.time.yesterday'))}
            </Text>
          ) : null}
        </View>
        <Text style={[styles.preview, last?.delivery_status === 'failed' && styles.failed]} numberOfLines={1}>
          {ticks(last)}
          {preview(last)}
        </Text>
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
    gap: spacing.md,
    backgroundColor: colors.ground,
  },
  pressed: { backgroundColor: colors.surface },
  body: {
    flex: 1,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.hairline,
    paddingBottom: spacing.md,
  },
  top: { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'baseline', gap: spacing.sm },
  name: { ...type.body, color: colors.text, fontWeight: '600', flexShrink: 1 },
  time: { ...type.caption, color: colors.textSecondary },
  preview: { ...type.secondary, color: colors.textSecondary, marginTop: 2 },
  failed: { color: colors.danger },
});
