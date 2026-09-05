import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { spacing, type, useTheme } from '@/theme';

import { Logo } from './Logo';

interface Props {
  title: string;
  // A word under the title, for a state worth knowing: "Connecting…".
  subtitle?: string | null;
  // The app's mark beside the title, on the screen that is the app.
  mark?: boolean;
  // Icon buttons, right-aligned.
  actions?: React.ReactNode;
}

// Header is the top of a tab screen: a large title on the left and its
// actions on the right, the way the apps people already use do it.
export function Header({ title, subtitle, mark, actions }: Props) {
  const { colors } = useTheme();
  return (
    <View style={styles.row}>
      {mark ? (
        <View style={styles.mark}>
          <Logo size={34} />
        </View>
      ) : null}
      <View style={styles.titles}>
        <Text style={[styles.title, { color: colors.text }]} accessibilityRole="header">
          {title}
        </Text>
        {subtitle ? <Text style={[styles.subtitle, { color: colors.textSecondary }]}>{subtitle}</Text> : null}
      </View>
      {actions ? <View style={styles.actions}>{actions}</View> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingLeft: spacing.lg,
    paddingRight: spacing.sm,
    paddingTop: spacing.md,
    paddingBottom: spacing.sm,
    minHeight: 60,
  },
  mark: { marginRight: spacing.sm + 2 },
  titles: { flex: 1 },
  title: type.display,
  subtitle: { ...type.secondary, marginTop: 2 },
  actions: { flexDirection: 'row', alignItems: 'center', gap: spacing.xs },
});
