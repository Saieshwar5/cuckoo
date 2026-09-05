import { Stack } from 'expo-router';
import React from 'react';

import { useTheme } from '@/theme';

// The agent screens: new, profile, edit, connect. Each draws its own bar.
export default function AgentLayout() {
  const { colors } = useTheme();
  return <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.ground } }} />;
}
