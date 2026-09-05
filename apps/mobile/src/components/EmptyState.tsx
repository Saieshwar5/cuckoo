import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { radius, spacing, type, useTheme } from '@/theme';

import { Button } from './Button';

interface Props {
  icon: keyof typeof Ionicons.glyphMap;
  title: string;
  subtitle: string;
  action?: { title: string; onPress: () => void };
}

// EmptyState fills a list that has nothing in it with what to do about it.
export function EmptyState({ icon, title, subtitle, action }: Props) {
  const { colors } = useTheme();
  return (
    <View style={styles.wrap}>
      <View style={[styles.disc, { backgroundColor: colors.accentTint }]}>
        <Ionicons name={icon} size={40} color={colors.accent} />
      </View>
      <Text style={[styles.title, { color: colors.text }]}>{title}</Text>
      <Text style={[styles.subtitle, { color: colors.textSecondary }]}>{subtitle}</Text>
      {action ? (
        <View style={styles.action}>
          <Button title={action.title} onPress={action.onPress} variant="outline" compact />
        </View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xxl, gap: spacing.sm },
  disc: {
    width: 88,
    height: 88,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: spacing.sm,
  },
  title: { ...type.title, textAlign: 'center' },
  subtitle: { ...type.body, textAlign: 'center', lineHeight: 22 },
  action: { marginTop: spacing.md },
});
