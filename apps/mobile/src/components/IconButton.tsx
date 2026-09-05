import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet } from 'react-native';

import { radius, tapTarget, useTheme } from '@/theme';

interface Props {
  icon: keyof typeof Ionicons.glyphMap;
  // What a screen reader says; there is no visible label.
  label: string;
  onPress: () => void;
  color?: string;
  size?: number;
  testID?: string;
}

// IconButton is a tap target round an icon: header actions, composer
// buttons, anything that is a picture rather than a word.
export function IconButton({ icon, label, onPress, color, size = 24, testID }: Props) {
  const { colors } = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      onPress={onPress}
      hitSlop={4}
      testID={testID}
      style={({ pressed }) => [styles.base, pressed && { backgroundColor: colors.surfaceStrong }]}
    >
      <Ionicons name={icon} size={size} color={color ?? colors.text} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    width: tapTarget,
    height: tapTarget,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
  },
});
