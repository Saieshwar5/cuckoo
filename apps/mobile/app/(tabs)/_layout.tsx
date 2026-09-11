import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import React from 'react';
import { StyleSheet, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useAgents } from '@/agents/AgentsProvider';
import { isMuted } from '@/agents/store';
import { unreadChats } from '@/chat/activity';
import { arrange } from '@/chats/store';
import { useChats } from '@/chats/useChats';
import { badge } from '@/components/ChatRow';
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
  const { conversations } = useChats();
  const { contacts } = useAgents();
  // The tab counts chats waiting to be read, not messages, and leaves out
  // what the person muted or set aside: a number that is always there is a
  // number nobody reads.
  const byAgent = new Map(contacts.map((c) => [c.agent.id, c]));
  const waiting = unreadChats(arrange(conversations, contacts).shown, (c) => {
    const agent = c.participants.find((p) => p.kind === 'agent');
    const contact = agent ? byAgent.get(agent.id) : undefined;
    return contact ? isMuted(contact) : false;
  });
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
          tabBarBadge: waiting ? badge(waiting) : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.accent, color: colors.onAccent, fontSize: 11 },
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
