import { Ionicons } from '@expo/vector-icons';
import React, { useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import type { ChatMessage } from '@/chat/store';
import { t } from '@/i18n';
import { inputReset, radius, sizes, spacing, type, useTheme } from '@/theme';

import { IconButton } from '../IconButton';

interface Props {
  replyTo: ChatMessage | null;
  agentName: string;
  onCancelReply: () => void;
  onSend: (text: string) => void;
}

// Composer is the bar at the bottom: what is being quoted, the words, and
// the one button. Enter sends in a browser, where a keyboard has a shift
// key for a new line; on a phone the button sends.
export function Composer({ replyTo, agentName, onCancelReply, onSend }: Props) {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  const [text, setText] = useState('');
  const ready = text.trim().length > 0;

  const submit = () => {
    if (!ready) return;
    onSend(text.trim());
    setText('');
  };

  return (
    <View style={[styles.bar, { backgroundColor: colors.ground, paddingBottom: spacing.sm + insets.bottom }]}>
      {replyTo ? (
        <View style={[styles.reply, { backgroundColor: colors.surface }]}>
          <View style={[styles.replyBar, { backgroundColor: colors.quoteBar }]} />
          <View style={styles.replyBody}>
            <Text style={[styles.replyName, { color: colors.quoteBar }]} numberOfLines={1}>
              {replyTo.sender.kind === 'user' ? t('chat.reply.you') : agentName}
            </Text>
            <Text style={[styles.replyText, { color: colors.textSecondary }]} numberOfLines={1}>
              {replyTo.body.text ?? ''}
            </Text>
          </View>
          <IconButton icon="close" label={t('chat.reply.cancel')} onPress={onCancelReply} size={20} />
        </View>
      ) : null}
      <View style={styles.row}>
        <View style={[styles.pill, { backgroundColor: colors.surface }]}>
          <TextInput
            accessibilityLabel={t('chat.placeholder')}
            testID="composer"
            value={text}
            onChangeText={setText}
            placeholder={t('chat.placeholder')}
            placeholderTextColor={colors.textSecondary}
            selectionColor={colors.accent}
            multiline
            onKeyPress={(e) => {
              if (Platform.OS !== 'web') return;
              const key = e.nativeEvent as unknown as { key: string; shiftKey?: boolean };
              if (key.key === 'Enter' && !key.shiftKey) {
                (e as unknown as { preventDefault?: () => void }).preventDefault?.();
                submit();
              }
            }}
            style={[styles.input, inputReset, { color: colors.text }]}
          />
        </View>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('chat.send')}
          accessibilityState={{ disabled: !ready }}
          disabled={!ready}
          onPress={submit}
          testID="send"
          style={({ pressed }) => [
            styles.send,
            { backgroundColor: ready ? colors.accent : colors.surfaceStrong },
            pressed && ready && styles.pressed,
          ]}
        >
          <Ionicons name="send" size={20} color={ready ? colors.onAccent : colors.textSecondary} />
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: { paddingHorizontal: spacing.sm, paddingTop: spacing.sm },
  reply: {
    flexDirection: 'row',
    alignItems: 'center',
    borderRadius: radius.md,
    overflow: 'hidden',
    marginBottom: spacing.sm,
    marginHorizontal: 2,
  },
  replyBar: { width: 4, alignSelf: 'stretch' },
  replyBody: { flex: 1, paddingHorizontal: spacing.sm + 2, paddingVertical: spacing.sm },
  replyName: { ...type.label, marginBottom: 1 },
  replyText: type.secondary,
  row: { flexDirection: 'row', alignItems: 'flex-end', gap: spacing.sm },
  pill: {
    flex: 1,
    minHeight: sizes.control,
    borderRadius: radius.xl,
    paddingHorizontal: spacing.lg,
    justifyContent: 'center',
  },
  input: { ...type.body, maxHeight: 120, paddingVertical: Platform.OS === 'ios' ? 12 : 10, lineHeight: 22 },
  send: {
    width: sizes.control,
    height: sizes.control,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
  },
  pressed: { opacity: 0.85 },
});
