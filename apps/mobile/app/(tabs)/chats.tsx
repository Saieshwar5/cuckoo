import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { filterConversations } from '@/chats/store';
import { useChats } from '@/chats/useChats';
import { ChatRow } from '@/components/ChatRow';
import { EmptyState } from '@/components/EmptyState';
import { Fab } from '@/components/Fab';
import { Header } from '@/components/Header';
import { Screen } from '@/components/Screen';
import { SearchBar } from '@/components/SearchBar';
import { t } from '@/i18n';
import { useTheme } from '@/theme';

// Chats: the home screen. The list is the hub's, kept live by the socket;
// the search box narrows it without asking the hub anything.
export default function ChatsScreen() {
  const { conversations, loading, refresh, connected } = useChats();
  const { colors } = useTheme();
  const router = useRouter();
  const [query, setQuery] = useState('');
  const shown = filterConversations(conversations, query);
  const goToAgents = () => router.push('/(tabs)/agents');

  return (
    <Screen padded={false}>
      <Header title={t('app.name')} mark subtitle={!connected && !loading ? t('chats.connecting') : null} />
      <SearchBar
        value={query}
        onChangeText={setQuery}
        placeholder={t('chats.search')}
        clearLabel={t('common.clear')}
      />
      <FlatList
        data={shown}
        keyExtractor={(c) => c.id}
        renderItem={({ item }) => (
          <ChatRow
            conversation={item}
            onPress={() => router.push({ pathname: '/chat/[id]', params: { id: item.id } })}
          />
        )}
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
              action={{ title: t('chats.empty.action'), onPress: goToAgents }}
            />
          )
        }
        contentContainerStyle={shown.length === 0 ? grow : padded}
        keyboardShouldPersistTaps="handled"
      />
      <Fab icon="chatbubble-ellipses" label={t('chats.new')} onPress={goToAgents} testID="new-chat" />
    </Screen>
  );
}

const grow = { flexGrow: 1 } as const;
// Room under the last row for the floating button.
const padded = { paddingBottom: 96 } as const;
