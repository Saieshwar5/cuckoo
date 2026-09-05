import React from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text } from 'react-native';

import { colors, radius, spacing, tapTarget, type } from '@/theme/tokens';

interface Props {
  title: string;
  onPress: () => void;
  disabled?: boolean;
  busy?: boolean;
  variant?: 'primary' | 'plain';
}

// Button: the accent for the one action that matters on a screen, plain
// for the rest.
export function Button({ title, onPress, disabled, busy, variant = 'primary' }: Props) {
  const primary = variant === 'primary';
  const inactive = disabled || busy;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: inactive }}
      onPress={onPress}
      disabled={inactive}
      style={({ pressed }) => [
        styles.base,
        primary ? styles.primary : styles.plain,
        inactive && styles.inactive,
        pressed && !inactive && styles.pressed,
      ]}
    >
      {busy ? (
        <ActivityIndicator color={primary ? colors.onAccent : colors.accent} />
      ) : (
        <Text style={[styles.label, primary ? styles.labelPrimary : styles.labelPlain]}>{title}</Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minHeight: tapTarget + 4,
    borderRadius: radius.md,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: spacing.lg,
  },
  primary: { backgroundColor: colors.accent },
  plain: { backgroundColor: 'transparent' },
  inactive: { opacity: 0.5 },
  pressed: { opacity: 0.85 },
  label: { ...type.body, fontWeight: '600' },
  labelPrimary: { color: colors.onAccent },
  labelPlain: { color: colors.accent },
});
