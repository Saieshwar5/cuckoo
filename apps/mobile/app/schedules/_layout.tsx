import { Stack } from 'expo-router';
import React from 'react';

import { useTheme } from '@/theme';

// The schedules screens: a conversation's list, and making or changing one.
export default function SchedulesLayout() {
  const { colors } = useTheme();
  return <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.ground } }} />;
}
