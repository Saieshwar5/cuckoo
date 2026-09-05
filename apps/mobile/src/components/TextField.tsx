import React from 'react';
import { StyleSheet, Text, TextInput, View, type TextInputProps } from 'react-native';

import { colors, radius, spacing, tapTarget, type } from '@/theme/tokens';

interface Props extends TextInputProps {
  label: string;
  error?: string | null;
}

// TextField: a label, an input, and the error under it when there is one.
export function TextField({ label, error, style, ...input }: Props) {
  return (
    <View style={styles.wrap}>
      <Text style={styles.label}>{label}</Text>
      <TextInput
        accessibilityLabel={label}
        placeholderTextColor={colors.textSecondary}
        style={[styles.input, error ? styles.inputError : null, style]}
        {...input}
      />
      {error ? <Text style={styles.error}>{error}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginBottom: spacing.lg },
  label: { ...type.secondary, color: colors.textSecondary, marginBottom: spacing.xs },
  input: {
    ...type.body,
    color: colors.text,
    minHeight: tapTarget + 4,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.hairline,
    borderRadius: radius.md,
    backgroundColor: colors.surface,
    paddingHorizontal: spacing.md,
  },
  inputError: { borderColor: colors.danger },
  error: { ...type.secondary, color: colors.danger, marginTop: spacing.xs },
});
