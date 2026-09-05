import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import type { Agent } from '@/api/types';
import { Avatar } from '@/components/Avatar';
import { Screen } from '@/components/Screen';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { colors, spacing, type } from '@/theme/tokens';

// Agents: the ones you own, with their connection state. Creating and
// connecting them is the next step; signing out lives here for now.
export default function AgentsScreen() {
  const { api, signOut } = useSession();
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  // State changes only inside the promise's callbacks: the effect itself
  // just starts the request, which is what an effect is for.
  const load = useCallback(
    () =>
      api.listAgents().then(
        (list) => {
          setAgents(list);
          setLoading(false);
        },
        () => {
          // The list stays as it was; pull to refresh tries again.
          setLoading(false);
        },
      ),
    [api],
  );

  useEffect(() => {
    load();
  }, [load]);

  return (
    <Screen padded={false}>
      <View style={styles.header}>
        <Text style={styles.title}>{t('agents.title')}</Text>
        <Pressable accessibilityRole="button" onPress={() => void signOut()} hitSlop={12}>
          <Text style={styles.signOut}>{t('settings.signout')}</Text>
        </Pressable>
      </View>
      <FlatList
        data={agents}
        keyExtractor={(a) => a.id}
        ListHeaderComponent={agents.length ? <Text style={styles.section}>{t('agents.yours')}</Text> : null}
        renderItem={({ item }) => (
          <View style={styles.row}>
            <Avatar name={item.display_name} status={item.binding?.status ?? null} />
            <View style={styles.body}>
              <Text style={styles.name}>{item.display_name}</Text>
              <Text style={styles.status}>
                @{item.handle} · {statusLabel(item)}
              </Text>
            </View>
          </View>
        )}
        refreshControl={
          <RefreshControl refreshing={loading} onRefresh={() => void load()} tintColor={colors.accent} />
        }
        ListEmptyComponent={
          loading ? null : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{t('agents.empty')}</Text>
            </View>
          )
        }
        contentContainerStyle={agents.length === 0 ? styles.flex : undefined}
      />
    </Screen>
  );
}

function statusLabel(a: Agent): string {
  return a.binding ? t(`agents.status.${a.binding.status}`) : t('agents.status.none');
}

const styles = StyleSheet.create({
  header: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.hairline,
  },
  title: { ...type.title, color: colors.text },
  signOut: { ...type.secondary, color: colors.accent },
  section: {
    ...type.caption,
    color: colors.textSecondary,
    paddingHorizontal: spacing.lg,
    paddingTop: spacing.lg,
    textTransform: 'uppercase',
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingHorizontal: spacing.lg,
    paddingVertical: spacing.md,
  },
  body: { flex: 1 },
  name: { ...type.body, color: colors.text, fontWeight: '600' },
  status: { ...type.secondary, color: colors.textSecondary, marginTop: 2 },
  flex: { flexGrow: 1 },
  empty: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl },
  emptyText: { ...type.body, color: colors.textSecondary, textAlign: 'center' },
});
