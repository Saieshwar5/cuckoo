import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { radius, spacing, type, useTheme } from '@/theme';

// DayDivider is the pill between one day's messages and the next.
export function DayDivider({ label }: { label: string }) {
  const { colors } = useTheme();
  return (
    <View style={styles.row}>
      <View style={[styles.pill, { backgroundColor: colors.surface }]}>
        <Text style={[styles.text, { color: colors.textSecondary }]}>{label}</Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { alignItems: 'center', marginVertical: spacing.sm },
  pill: { paddingHorizontal: spacing.md, paddingVertical: spacing.xs + 1, borderRadius: radius.sm },
  text: type.caption,
});
