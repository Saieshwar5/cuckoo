import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import { isMuted } from '@/agents/store';
import { arrange, filterConversations } from '@/chats/store';
import { useChats } from '@/chats/useChats';
import { ChatRow } from '@/components/ChatRow';
import { EmptyState } from '@/components/EmptyState';
import { Header } from '@/components/Header';
import { IconButton } from '@/components/IconButton';
import { Screen } from '@/components/Screen';
import { SearchBar } from '@/components/SearchBar';
import { t } from '@/i18n';
import { sizes, spacing, type, useTheme } from '@/theme';

// Chats: the home screen. The list is the hub's, kept live by the socket;
// the search box narrows it without asking the hub anything. What the
// person decided about each agent — pinned, muted, archived — comes from
// their contacts and is laid over the list here.
export default function ChatsScreen() {
  const { conversations, loading, refresh, connected } = useChats();
  const { contacts } = useAgents();
  const { colors } = useTheme();
  const router = useRouter();
  const [query, setQuery] = useState('');
  const arranged = arrange(conversations, contacts);
  const shown = filterConversations(arranged.shown, query);
  const byAgent = new Map(contacts.map((c) => [c.agent.id, c]));
  const contactOf = (c: (typeof conversations)[number]) => {
    const agent = c.participants.find((p) => p.kind === 'agent');
    return agent ? byAgent.get(agent.id) : undefined;
  };

  return (
    <Screen padded={false}>
      <Header
        title={t('app.name')}
        mark
        subtitle={!connected && !loading ? t('chats.connecting') : null}
        actions={
          <>
            {/* Scanning lives in the top bar, beside search, because that is
                where UPI taught every thumb in India to look for it. */}
            <IconButton
              icon="qr-code-outline"
              label={t('chats.new.scan')}
              onPress={() => router.push('/scan')}
              testID="chats-scan"
            />
            <IconButton
              icon="sparkles-outline"
              label={t('catalogue.title')}
              onPress={() => router.push('/catalogue')}
              testID="chats-catalogue"
            />
          </>
        }
      />
      <SearchBar
        value={query}
        onChangeText={setQuery}
        placeholder={t('chats.search')}
        clearLabel={t('common.clear')}
      />
      <FlatList
        data={shown}
        keyExtractor={(c) => c.id}
        renderItem={({ item }) => {
          const contact = contactOf(item);
          return (
            <ChatRow
              conversation={item}
              pinned={contact?.pinned}
              muted={contact ? isMuted(contact) : false}
              onPress={() => router.push({ pathname: '/chat/[id]', params: { id: item.id } })}
            />
          );
        }}
        ListHeaderComponent={
          arranged.archived.length && !query ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push('/archived')}
              style={({ pressed }) => [styles.archived, pressed && { backgroundColor: colors.surface }]}
              testID="archived-row"
            >
              <View style={[styles.archivedIcon, { backgroundColor: colors.surface }]}>
                <Ionicons name="archive-outline" size={20} color={colors.accent} />
              </View>
              <Text style={[styles.archivedLabel, { color: colors.text }]}>{t('chats.archived')}</Text>
              <Text style={[styles.archivedCount, { color: colors.textSecondary }]}>
                {arranged.archived.length}
              </Text>
            </Pressable>
          ) : null
        }
        refreshControl={
          <RefreshControl refreshing={loading} onRefresh={() => void refresh()} tintColor={colors.accent} />
        }
        ListEmptyComponent={
          loading ? null : query ? (
            <EmptyState
              icon="search"
              title={t('chats.nomatch.title')}
              subtitle={t('chats.nomatch.subtitle')}
            />
          ) : (
            <EmptyState
              icon="chatbubbles-outline"
              title={t('chats.empty.title')}
              subtitle={t('chats.empty.subtitle')}
              action={{ title: t('agents.empty.browse'), onPress: () => router.push('/catalogue') }}
              secondary={{ title: t('chats.empty.scan'), onPress: () => router.push('/scan') }}
            />
          )
        }
        contentContainerStyle={shown.length === 0 ? grow : padded}
        keyboardShouldPersistTaps="handled"
      />
    </Screen>
  );
}

const grow = { flexGrow: 1 } as const;
// Room under the last row for the floating button.
const padded = { paddingBottom: 96 } as const;

const styles = StyleSheet.create({
  // Drawn like a row, so the eye reads it as part of the list.
  archived: {
    flexDirection: 'row',
    alignItems: 'center',
    paddingVertical: spacing.sm + 2,
    paddingHorizontal: spacing.lg,
    gap: spacing.md + 2,
  },
  archivedIcon: {
    width: sizes.avatar,
    height: sizes.avatar,
    borderRadius: sizes.avatar / 2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  archivedLabel: { ...type.headline, flex: 1 },
  archivedCount: type.secondary,
});
