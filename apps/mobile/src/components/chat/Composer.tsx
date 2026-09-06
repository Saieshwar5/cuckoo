import { Ionicons } from '@expo/vector-icons';
import React, { useState } from 'react';
import { Image, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import type { ChatMessage } from '@/chat/store';
import { t } from '@/i18n';
import { formatBytes } from '@/media/format';
import { MAX_FILES, pickDocuments, pickPhotos, takePhoto, type PickedFile } from '@/media/pick';
import { inputReset, radius, sizes, spacing, type, useTheme } from '@/theme';

import { ActionSheet } from '../ActionSheet';
import { IconButton } from '../IconButton';

interface Props {
  replyTo: ChatMessage | null;
  agentName: string;
  onCancelReply: () => void;
  onSend: (text: string, files: PickedFile[]) => void;
}

// Composer is the bar at the bottom: what is being quoted, the words, and
// the one button. Enter sends in a browser, where a keyboard has a shift
// key for a new line; on a phone the button sends.
export function Composer({ replyTo, agentName, onCancelReply, onSend }: Props) {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  const [text, setText] = useState('');
  const [files, setFiles] = useState<PickedFile[]>([]);
  const [picking, setPicking] = useState(false);
  // A photo with nothing written under it is a message; an empty one is not.
  const ready = text.trim().length > 0 || files.length > 0;

  const submit = () => {
    if (!ready) return;
    onSend(text.trim(), files);
    setText('');
    setFiles([]);
  };

  const add = async (pick: () => Promise<PickedFile[]>) => {
    const picked = await pick();
    if (picked.length) setFiles((current) => [...current, ...picked].slice(0, MAX_FILES));
  };

  return (
    <View style={[styles.bar, { backgroundColor: colors.ground, paddingBottom: spacing.sm + insets.bottom }]}>
      {replyTo ? (
        <View style={[styles.reply, { backgroundColor: colors.surface }]}>
          <View style={[styles.replyBar, { backgroundColor: colors.accent }]} />
          <View style={styles.replyBody}>
            <Text style={[styles.replyName, { color: colors.accent }]} numberOfLines={1}>
              {replyTo.sender.kind === 'user' ? t('chat.reply.you') : agentName}
            </Text>
            <Text style={[styles.replyText, { color: colors.textSecondary }]} numberOfLines={1}>
              {replyTo.body.text ?? ''}
            </Text>
          </View>
          <IconButton icon="close" label={t('chat.reply.cancel')} onPress={onCancelReply} size={20} />
        </View>
      ) : null}
      {files.length ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.picked}
          style={styles.pickedRow}
        >
          {files.map((f, i) => (
            <View key={`${f.uri}-${i}`} style={[styles.chip, { backgroundColor: colors.surface }]}>
              {f.kind === 'image' ? (
                <Image source={{ uri: f.uri }} style={styles.chipImage} accessibilityIgnoresInvertColors />
              ) : (
                <View style={[styles.chipImage, styles.chipIcon, { backgroundColor: colors.surfaceStrong }]}>
                  <Ionicons name="document-text" size={18} color={colors.textSecondary} />
                </View>
              )}
              <View style={styles.chipBody}>
                <Text style={[styles.chipName, { color: colors.text }]} numberOfLines={1}>
                  {f.name}
                </Text>
                {f.byteSize ? (
                  <Text style={[styles.chipSize, { color: colors.textSecondary }]}>
                    {formatBytes(f.byteSize)}
                  </Text>
                ) : null}
              </View>
              <IconButton
                icon="close"
                label={t('chat.attach.remove')}
                size={18}
                testID={`unattach-${i}`}
                onPress={() => setFiles((current) => current.filter((_, j) => j !== i))}
              />
            </View>
          ))}
        </ScrollView>
      ) : null}
      <View style={styles.row}>
        <IconButton
          icon="add-circle-outline"
          label={t('chat.attach')}
          size={26}
          testID="attach"
          onPress={() => setPicking(true)}
        />
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
      <ActionSheet
        visible={picking}
        onClose={() => setPicking(false)}
        actions={[
          {
            icon: 'images-outline',
            label: t('chat.attach.photos'),
            testID: 'attach-photos',
            onPress: () => void add(pickPhotos),
          },
          {
            icon: 'camera-outline',
            label: t('chat.attach.camera'),
            testID: 'attach-camera',
            onPress: () => void add(takePhoto),
          },
          {
            icon: 'document-outline',
            label: t('chat.attach.file'),
            testID: 'attach-file',
            onPress: () => void add(pickDocuments),
          },
        ]}
      />
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
  row: { flexDirection: 'row', alignItems: 'flex-end', gap: spacing.xs },
  pickedRow: { flexGrow: 0, marginBottom: spacing.sm },
  picked: { gap: spacing.sm, paddingHorizontal: 2 },
  chip: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    borderRadius: radius.md,
    padding: spacing.xs + 2,
    paddingRight: spacing.xs,
    maxWidth: 240,
  },
  chipImage: { width: 34, height: 34, borderRadius: radius.sm },
  chipIcon: { alignItems: 'center', justifyContent: 'center' },
  chipBody: { flexShrink: 1, gap: 1 },
  chipName: { ...type.caption, fontWeight: '600' },
  chipSize: type.caption,
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
