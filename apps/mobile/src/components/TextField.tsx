import { Ionicons } from '@expo/vector-icons';
import React, { useState } from 'react';
import { StyleSheet, Text, TextInput, View, type TextInputProps } from 'react-native';

import { inputReset, radius, sizes, spacing, type, useTheme } from '@/theme';

interface Props extends TextInputProps {
  label: string;
  error?: string | null;
  icon?: keyof typeof Ionicons.glyphMap;
}

// TextField: a label, a filled input that lights up while it has focus, and
// the error under it when there is one.
export function TextField({ label, error, icon, style, onFocus, onBlur, ...input }: Props) {
  const { colors } = useTheme();
  const [focused, setFocused] = useState(false);
  const border = error ? colors.danger : focused ? colors.accent : colors.surface;
  return (
    <View style={styles.wrap}>
      <Text style={[styles.label, { color: colors.textSecondary }]}>{label}</Text>
      <View style={[styles.field, { backgroundColor: colors.surface, borderColor: border }]}>
        {icon ? (
          <Ionicons name={icon} size={20} color={focused ? colors.accent : colors.textSecondary} />
        ) : null}
        <TextInput
          accessibilityLabel={label}
          placeholderTextColor={colors.textSecondary}
          selectionColor={colors.accent}
          style={[styles.input, inputReset, { color: colors.text }, style]}
          onFocus={(e) => {
            setFocused(true);
            onFocus?.(e);
          }}
          onBlur={(e) => {
            setFocused(false);
            onBlur?.(e);
          }}
          {...input}
        />
      </View>
      {error ? <Text style={[styles.error, { color: colors.danger }]}>{error}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginBottom: spacing.lg },
  label: { ...type.label, marginBottom: spacing.sm },
  field: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    minHeight: sizes.control + 4,
    borderWidth: 1.5,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
  },
  input: { ...type.body, flex: 1, minHeight: sizes.control, paddingVertical: 0 },
  error: { ...type.secondary, marginTop: spacing.sm },
});
