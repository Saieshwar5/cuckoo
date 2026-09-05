import { useRouter } from 'expo-router';
import React from 'react';
import { FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { useChats } from '@/chats/useChats';
import { ChatRow } from '@/components/ChatRow';
import { Screen } from '@/components/Screen';
import { t } from '@/i18n';
import { colors, spacing, type } from '@/theme/tokens';

// Chats: the home screen. The list is the hub's, kept live by the socket.
export default function ChatsScreen() {
  const { conversations, loading, refresh, connected } = useChats();
  const router = useRouter();

  return (
    <Screen padded={false}>
      <View style={styles.header}>
        <Text style={styles.title}>{t('chats.title')}</Text>
        {!connected && !loading ? <View style={styles.offline} testID="offline-dot" /> : null}
      </View>
      <FlatList
        data={conversations}
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
          loading ? null : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{t('chats.empty')}</Text>
            </View>
          )
        }
        contentContainerStyle={conversations.length === 0 ? styles.flex : undefined}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.hairline,
  },
  title: { ...type.title, color: colors.text },
  offline: { width: 8, height: 8, borderRadius: 4, backgroundColor: colors.statusUnreachable },
  flex: { flexGrow: 1 },
  empty: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl },
  emptyText: { ...type.body, color: colors.textSecondary, textAlign: 'center' },
});
