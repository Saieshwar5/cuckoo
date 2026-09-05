import React from 'react';
import { Pressable, ScrollView, StyleSheet, Text } from 'react-native';

import { t } from '@/i18n';
import { radius, spacing, type, useTheme } from '@/theme';

// QuickReplies are the agent's suggested answers, as chips above the
// composer. A tap sends the words as they are.
export function QuickReplies({ labels, onPick }: { labels: string[]; onPick: (label: string) => void }) {
  const { colors } = useTheme();
  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      keyboardShouldPersistTaps="handled"
      style={styles.scroll}
      contentContainerStyle={styles.row}
      accessibilityLabel={t('chat.quick.label')}
      testID="quick-replies"
    >
      {labels.map((label) => (
        <Pressable
          key={label}
          accessibilityRole="button"
          onPress={() => onPick(label)}
          style={({ pressed }) => [
            styles.chip,
            { borderColor: colors.accent, backgroundColor: pressed ? colors.accentTint : colors.ground },
          ]}
        >
          <Text style={[styles.label, { color: colors.accent }]}>{label}</Text>
        </Pressable>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  // A scroll view grows to fill its parent unless told not to.
  scroll: { flexGrow: 0 },
  row: { flexDirection: 'row', gap: spacing.sm, paddingHorizontal: spacing.md, paddingVertical: spacing.sm },
  chip: {
    borderWidth: 1,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.md + 2,
    paddingVertical: spacing.sm,
    minHeight: 36,
    justifyContent: 'center',
  },
  label: { ...type.secondary, fontWeight: '600' },
});
