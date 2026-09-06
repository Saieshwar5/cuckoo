import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, Text } from 'react-native';

import type { AgentStatus } from '@/api/types';
import { radius, spacing, type, type Palette } from '@/theme';

export function statusColor(colors: Palette, status: AgentStatus | null): string {
  switch (status) {
    case 'connected':
      return colors.statusConnected;
    case 'unreachable':
      return colors.statusUnreachable;
    default:
      return colors.statusIdle;
  }
}

// ActionTile is one of the pill cards under the name, as the reference
// apps draw a contact's actions.
export function ActionTile({
  icon,
  label,
  onPress,
  disabled,
  colors,
  testID,
}: {
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  onPress: () => void;
  disabled?: boolean;
  colors: Palette;
  testID: string;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled }}
      disabled={disabled}
      onPress={onPress}
      testID={testID}
      style={({ pressed }) => [
        tile.base,
        { backgroundColor: pressed ? colors.surfaceStrong : colors.surface },
        disabled && tile.disabled,
      ]}
    >
      <Ionicons name={icon} size={22} color={colors.accent} />
      <Text style={[tile.label, { color: colors.text }]}>{label}</Text>
    </Pressable>
  );
}

const tile = StyleSheet.create({
  base: {
    flex: 1,
    alignItems: 'center',
    gap: spacing.xs,
    paddingVertical: spacing.md,
    borderRadius: radius.lg,
  },
  label: type.label,
  disabled: { opacity: 0.5 },
});
