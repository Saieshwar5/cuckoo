import React from 'react';
import { Modal, Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { t } from '@/i18n';
import { radius, spacing, type, useTheme } from '@/theme';

import { Button } from './Button';

interface Props {
  visible: boolean;
  title: string;
  body: string;
  confirmLabel: string;
  // Destructive confirmations are red; the rest use the accent.
  destructive?: boolean;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

// ConfirmSheet asks once before something that cannot be undone. A sheet
// from the bottom, the same on every platform, rather than the system alert
// the web does not have.
export function ConfirmSheet({
  visible,
  title,
  body,
  confirmLabel,
  destructive,
  busy,
  onConfirm,
  onCancel,
}: Props) {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onCancel}>
      <Pressable style={styles.backdrop} onPress={onCancel} accessibilityLabel={t('common.cancel')} />
      <View
        style={[styles.sheet, { backgroundColor: colors.ground, paddingBottom: spacing.lg + insets.bottom }]}
        testID="confirm-sheet"
      >
        <Text style={[styles.title, { color: colors.text }]}>{title}</Text>
        <Text style={[styles.body, { color: colors.textSecondary }]}>{body}</Text>
        <View style={styles.buttons}>
          <Button title={t('common.cancel')} onPress={onCancel} variant="outline" disabled={busy} />
          <Button
            title={confirmLabel}
            onPress={onConfirm}
            tone={destructive ? 'danger' : 'accent'}
            busy={busy}
            testID="confirm"
          />
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: 'rgba(0, 0, 0, 0.5)' },
  sheet: {
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.xl,
    borderTopLeftRadius: radius.xl,
    borderTopRightRadius: radius.xl,
    gap: spacing.sm,
  },
  title: type.title,
  body: { ...type.body, lineHeight: 22 },
  buttons: { gap: spacing.sm, marginTop: spacing.md },
});
