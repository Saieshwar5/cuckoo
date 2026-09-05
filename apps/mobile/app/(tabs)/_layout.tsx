import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import React from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { t } from '@/i18n';
import { radius, type, useTheme } from '@/theme';

// Chats is the home tab.
export const unstable_settings = { initialRouteName: 'chats' };

type TabIconName = 'chatbubbles' | 'people';

// TabIcon is an icon that sits in a tinted pill while its tab is open, the
// way Android's own apps mark the current tab.
function TabIcon({ name, focused }: { name: TabIconName; focused: boolean }) {
  const { colors } = useTheme();
  const glyph: keyof typeof Ionicons.glyphMap = focused ? name : `${name}-outline`;
  return (
    <View style={[styles.pill, focused && { backgroundColor: colors.accentTint }]}>
      <Ionicons name={glyph} size={24} color={focused ? colors.accentStrong : colors.textSecondary} />
    </View>
  );
}

// Two tabs, like the chat apps people already know.
export default function TabsLayout() {
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarActiveTintColor: colors.text,
        tabBarInactiveTintColor: colors.textSecondary,
        tabBarStyle: {
          backgroundColor: colors.ground,
          borderTopColor: colors.hairline,
          height: 66 + insets.bottom,
          paddingTop: 8,
        },
        tabBarLabelStyle: { ...type.caption, fontWeight: '600' },
        sceneStyle: { backgroundColor: colors.ground },
      }}
    >
      <Tabs.Screen
        name="chats"
        options={{
          title: t('tabs.chats'),
          tabBarIcon: ({ focused }) => <TabIcon name="chatbubbles" focused={focused} />,
        }}
      />
      <Tabs.Screen
        name="agents"
        options={{
          title: t('tabs.agents'),
          tabBarIcon: ({ focused }) => <TabIcon name="people" focused={focused} />,
        }}
      />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  pill: { width: 56, height: 32, borderRadius: radius.pill, alignItems: 'center', justifyContent: 'center' },
});
