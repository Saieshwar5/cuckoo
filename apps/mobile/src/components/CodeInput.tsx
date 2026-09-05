import React, { useRef, useState } from 'react';
import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native';

import { inputReset, radius, spacing, type, useTheme } from '@/theme';

interface Props {
  label: string;
  value: string;
  onChange: (value: string) => void;
  // Called with the whole code the moment the last digit lands, so the
  // person never has to press anything after typing.
  onComplete?: (value: string) => void;
  length?: number;
  error?: string | null;
  autoFocus?: boolean;
  testID?: string;
}

// CodeInput shows a code as one box per digit while one invisible input
// underneath does the typing. The keyboard and autofill see an ordinary
// one-time-code field; the person sees six boxes filling up.
export function CodeInput({
  label,
  value,
  onChange,
  onComplete,
  length = 6,
  error,
  autoFocus,
  testID,
}: Props) {
  const { colors } = useTheme();
  const input = useRef<TextInput>(null);
  const [focused, setFocused] = useState(false);
  const digits = value.replace(/\D/g, '').slice(0, length);

  const change = (raw: string) => {
    const next = raw.replace(/\D/g, '').slice(0, length);
    onChange(next);
    if (next.length === length && next !== digits) onComplete?.(next);
  };

  return (
    <View style={styles.wrap}>
      <Pressable onPress={() => input.current?.focus()} style={styles.boxes} accessible={false}>
        {Array.from({ length }, (_, i) => {
          const active = focused && i === Math.min(digits.length, length - 1);
          const border = error ? colors.danger : active ? colors.accent : colors.surface;
          return (
            <View key={i} style={[styles.box, { backgroundColor: colors.surface, borderColor: border }]}>
              <Text style={[styles.digit, { color: colors.text }]}>{digits[i] ?? ''}</Text>
              {active && digits.length < length ? (
                <View style={[styles.caret, { backgroundColor: colors.accent }]} />
              ) : null}
            </View>
          );
        })}
      </Pressable>
      <TextInput
        ref={input}
        accessibilityLabel={label}
        testID={testID}
        value={digits}
        onChangeText={change}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        keyboardType="number-pad"
        inputMode="numeric"
        textContentType="oneTimeCode"
        autoComplete="one-time-code"
        maxLength={length}
        autoFocus={autoFocus}
        caretHidden
        style={[styles.hidden, inputReset]}
      />
      {error ? <Text style={[styles.error, { color: colors.danger }]}>{error}</Text> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { marginBottom: spacing.lg },
  boxes: { flexDirection: 'row', justifyContent: 'space-between', gap: spacing.sm },
  box: {
    flex: 1,
    height: 56,
    borderRadius: radius.md,
    borderWidth: 1.5,
    alignItems: 'center',
    justifyContent: 'center',
  },
  digit: { ...type.title, fontVariant: ['tabular-nums'] },
  caret: { position: 'absolute', width: 2, height: 24, borderRadius: 1 },
  // Present for the keyboard, invisible to the eye, and over the boxes so
  // a tap anywhere on them lands in it.
  hidden: { position: 'absolute', top: 0, left: 0, right: 0, height: 56, opacity: 0 },
  error: { ...type.secondary, marginTop: spacing.sm },
});
