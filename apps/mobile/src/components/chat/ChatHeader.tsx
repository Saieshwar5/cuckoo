import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import type { AgentStatus } from '@/api/types';
import { t } from '@/i18n';
import type { MediaSource } from '@/media/source';
import { sizes, spacing, type, useTheme } from '@/theme';

import { Avatar } from '../Avatar';
import { IconButton } from '../IconButton';

interface Props {
  name: string;
  status: AgentStatus | null | undefined;
  typing: boolean;
  avatar?: MediaSource | null;
  onBack: () => void;
}

// ChatHeader names who the conversation is with and what they are doing:
// online, typing, or out of reach.
export function ChatHeader({ name, status, typing, avatar, onBack }: Props) {
  const { colors } = useTheme();
  const line = typing
    ? t('chat.typing')
    : status === undefined
      ? ''
      : status
        ? t(`chat.status.${status}`)
        : t('chat.status.none');
  return (
    <View style={[styles.bar, { backgroundColor: colors.surface }]}>
      <IconButton icon="arrow-back" label={t('chat.back')} onPress={onBack} testID="chat-back" />
      <Avatar name={name || '?'} size={sizes.avatarSmall} status={status} source={avatar} />
      <View style={styles.titles}>
        <Text style={[styles.name, { color: colors.text }]} numberOfLines={1}>
          {name}
        </Text>
        {line ? (
          <Text
            style={[styles.status, { color: typing ? colors.accentStrong : colors.textSecondary }]}
            numberOfLines={1}
            testID="chat-status"
          >
            {line}
          </Text>
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingLeft: spacing.xs,
    paddingRight: spacing.md,
    paddingVertical: spacing.sm,
    minHeight: 60,
  },
  titles: { flex: 1, marginLeft: spacing.xs },
  name: type.headline,
  status: { ...type.caption, marginTop: 1 },
});
