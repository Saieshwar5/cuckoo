import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, TextInput, View } from 'react-native';

import { inputReset, radius, spacing, type, useTheme } from '@/theme';

interface Props {
  value: string;
  onChangeText: (value: string) => void;
  placeholder: string;
  // What a screen reader calls the clear button.
  clearLabel: string;
}

// SearchBar is the pill under a list's title. It filters what is already on
// the screen; nothing is asked of the hub.
export function SearchBar({ value, onChangeText, placeholder, clearLabel }: Props) {
  const { colors } = useTheme();
  return (
    <View style={[styles.pill, { backgroundColor: colors.surface }]}>
      <Ionicons name="search" size={20} color={colors.textSecondary} />
      <TextInput
        accessibilityLabel={placeholder}
        value={value}
        onChangeText={onChangeText}
        placeholder={placeholder}
        placeholderTextColor={colors.textSecondary}
        selectionColor={colors.accent}
        autoCapitalize="none"
        autoCorrect={false}
        returnKeyType="search"
        style={[styles.input, inputReset, { color: colors.text }]}
      />
      {value ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={clearLabel}
          onPress={() => onChangeText('')}
          hitSlop={8}
        >
          <Ionicons name="close-circle" size={20} color={colors.textSecondary} />
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  pill: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    marginHorizontal: spacing.lg,
    marginBottom: spacing.sm,
    paddingHorizontal: spacing.md,
    height: 46,
    borderRadius: radius.pill,
  },
  input: { ...type.body, flex: 1, paddingVertical: 0 },
});
