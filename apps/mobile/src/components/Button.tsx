import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { radius, sizes, spacing, type, useTheme } from '@/theme';

type Icon = keyof typeof Ionicons.glyphMap;

interface Props {
  title: string;
  onPress: () => void;
  disabled?: boolean;
  busy?: boolean;
  // primary is the one filled pill on a screen; outline and plain are for
  // the rest.
  variant?: 'primary' | 'outline' | 'plain';
  icon?: Icon;
  // compact hugs its label instead of filling the row.
  compact?: boolean;
  testID?: string;
}

export function Button({
  title,
  onPress,
  disabled,
  busy,
  variant = 'primary',
  icon,
  compact,
  testID,
}: Props) {
  const { colors } = useTheme();
  const inactive = disabled || busy;
  // A disabled primary goes quiet rather than translucent: a faded accent
  // reads as broken, a grey pill reads as "not yet".
  const fg =
    variant === 'primary' ? (disabled && !busy ? colors.textSecondary : colors.onAccent) : colors.accent;
  const bg =
    variant === 'primary' ? (disabled && !busy ? colors.surfaceStrong : colors.accent) : 'transparent';
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: inactive }}
      onPress={onPress}
      disabled={inactive}
      testID={testID}
      style={({ pressed }) => [
        styles.base,
        compact && styles.compact,
        { backgroundColor: bg },
        variant === 'outline' && { borderWidth: 1, borderColor: colors.accent },
        pressed && !inactive && styles.pressed,
      ]}
    >
      {busy ? (
        <ActivityIndicator color={fg} />
      ) : (
        <View style={styles.content}>
          {icon ? <Ionicons name={icon} size={18} color={fg} /> : null}
          <Text style={[styles.label, { color: fg }]}>{title}</Text>
        </View>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minHeight: sizes.control,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
    paddingHorizontal: spacing.xl,
  },
  compact: { alignSelf: 'center', minHeight: 40, paddingHorizontal: spacing.lg },
  pressed: { opacity: 0.8 },
  content: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  label: { ...type.body, fontWeight: '600' },
});
