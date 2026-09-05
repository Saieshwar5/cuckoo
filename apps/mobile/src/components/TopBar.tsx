import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { t } from '@/i18n';
import { spacing, type, useTheme } from '@/theme';

import { IconButton } from './IconButton';

interface Props {
  title: string;
  onBack: () => void;
  // An icon button on the right, if the screen has one action.
  action?: React.ReactNode;
}

// TopBar is the top of a stack screen: back, a title, at most one action.
export function TopBar({ title, onBack, action }: Props) {
  const { colors } = useTheme();
  return (
    <View style={[styles.bar, { backgroundColor: colors.ground, borderBottomColor: colors.hairline }]}>
      <IconButton icon="arrow-back" label={t('common.back')} onPress={onBack} testID="back" />
      <Text style={[styles.title, { color: colors.text }]} numberOfLines={1} accessibilityRole="header">
        {title}
      </Text>
      <View style={styles.action}>{action}</View>
    </View>
  );
}

const styles = StyleSheet.create({
  bar: {
    flexDirection: 'row',
    alignItems: 'center',
    minHeight: 56,
    paddingHorizontal: spacing.xs,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  title: { ...type.headline, flex: 1, marginLeft: spacing.xs },
  action: { minWidth: 44, alignItems: 'flex-end' },
});
