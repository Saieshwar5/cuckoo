import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import type { AgentStatus } from '@/api/types';
import { t } from '@/i18n';
import type { MediaSource } from '@/media/source';
import { sizes, spacing, type, useTheme } from '@/theme';

import { Avatar } from '../Avatar';
import { IconButton } from '../IconButton';
import { WorkingDots } from '../WorkingDots';

interface Props {
  name: string;
  status: AgentStatus | null | undefined;
  // What the agent is doing, in words — "thinking…", "checking the
  // weather…" — or null when it is not busy.
  activity: string | null;
  avatar?: MediaSource | null;
  onBack: () => void;
  /** Opens the agent's profile. Ten years of messengers have trained the
   *  thumb to tap the name at the top to find out who this is. */
  onOpenProfile?: () => void;
  /** The short menu of things somebody wants without leaving the chat. */
  onMore?: () => void;
}

// ChatHeader names who the conversation is with and what they are doing:
// online, busy at something it names, or out of reach.
//
// The name and picture are a button, because that is where everybody looks
// for "who is this". Beside them is a menu holding the two or three things
// somebody reaches for while they are annoyed — muting, most of all. If mute
// is three screens away nobody mutes; they block, and a block is permanent.
export function ChatHeader({ name, status, activity, avatar, onBack, onOpenProfile, onMore }: Props) {
  const { colors } = useTheme();
  const line = activity
    ? activity
    : status === undefined
      ? ''
      : status
        ? t(`chat.status.${status}`)
        : t('chat.status.none');
  return (
    <View style={[styles.bar, { backgroundColor: colors.surface }]}>
      <IconButton icon="arrow-back" label={t('chat.back')} onPress={onBack} testID="chat-back" />
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('chat.openProfile', { name })}
        onPress={onOpenProfile}
        disabled={!onOpenProfile}
        style={styles.who}
        testID="chat-open-profile"
      >
        <Avatar name={name || '?'} size={sizes.avatarSmall} status={status} source={avatar} />
        <View style={styles.titles}>
          <Text style={[styles.name, { color: colors.text }]} numberOfLines={1}>
            {name}
          </Text>
          {line ? (
            <View style={styles.statusRow}>
              {activity ? <WorkingDots color={colors.accentStrong} size={4} /> : null}
              <Text
                style={[styles.status, { color: activity ? colors.accentStrong : colors.textSecondary }]}
                numberOfLines={1}
                testID="chat-status"
              >
                {line}
              </Text>
            </View>
          ) : null}
        </View>
      </Pressable>
      {onMore ? (
        <IconButton icon="ellipsis-vertical" label={t('chat.more')} onPress={onMore} testID="chat-more" />
      ) : null}
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
  // The whole name-and-picture block is one target, so a thumb aiming at
  // either lands on the profile.
  who: { flex: 1, flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  titles: { flex: 1, marginLeft: spacing.xs },
  name: type.headline,
  statusRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.xs, marginTop: 1 },
  status: { ...type.caption, flexShrink: 1 },
});
