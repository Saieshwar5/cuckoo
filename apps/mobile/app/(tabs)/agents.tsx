import { useRouter } from 'expo-router';
import React from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import type { AgentStatus } from '@/api/types';
import { Avatar } from '@/components/Avatar';
import { EmptyState } from '@/components/EmptyState';
import { Fab } from '@/components/Fab';
import { Header } from '@/components/Header';
import { IconButton } from '@/components/IconButton';
import { Screen } from '@/components/Screen';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';

// Agents: the ones you own, with their connection state, live. Tap one for
// its profile; the button makes a new one. Signing out lives here for now.
export default function AgentsScreen() {
  const { signOut } = useSession();
  const { agents, loading, controller } = useAgents();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const create = () => router.push('/agent/new');

  return (
    <Screen padded={false}>
      <Header
        title={t('agents.title')}
        actions={
          <IconButton icon="log-out-outline" label={t('settings.signout')} onPress={() => void signOut()} />
        }
      />
      <FlatList
        data={agents}
        keyExtractor={(a) => a.id}
        ListHeaderComponent={agents.length ? <Text style={styles.section}>{t('agents.yours')}</Text> : null}
        renderItem={({ item }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push({ pathname: '/agent/[id]', params: { id: item.id } })}
            style={({ pressed }) => [styles.row, pressed && { backgroundColor: colors.surface }]}
            testID={`agent-row-${item.id}`}
          >
            <Avatar name={item.display_name} status={item.binding?.status ?? null} />
            <View style={styles.body}>
              <Text style={styles.name} numberOfLines={1}>
                {item.display_name}
              </Text>
              <Text style={styles.handle} numberOfLines={1}>
                @{item.handle}
              </Text>
            </View>
            <StatusChip status={item.binding?.status ?? null} colors={colors} />
          </Pressable>
        )}
        refreshControl={
          <RefreshControl
            refreshing={loading}
            onRefresh={() => void controller?.refresh()}
            tintColor={colors.accent}
          />
        }
        ListEmptyComponent={
          loading ? null : (
            <EmptyState
              icon="sparkles-outline"
              title={t('agents.empty.title')}
              subtitle={t('agents.empty.subtitle')}
              action={{ title: t('agents.empty.action'), onPress: create }}
            />
          )
        }
        contentContainerStyle={agents.length === 0 ? styles.grow : styles.padded}
      />
      <Fab icon="add" label={t('agents.new')} onPress={create} testID="new-agent" />
    </Screen>
  );
}

function statusColor(colors: Palette, status: AgentStatus | null): string {
  switch (status) {
    case 'connected':
      return colors.statusConnected;
    case 'unreachable':
      return colors.statusUnreachable;
    default:
      return colors.statusIdle;
  }
}

// StatusChip says in a word what the avatar's dot says in a colour.
function StatusChip({ status, colors }: { status: AgentStatus | null; colors: Palette }) {
  const tint = statusColor(colors, status);
  return (
    <View style={[chip.wrap, { backgroundColor: colors.surface }]}>
      <View style={[chip.dot, { backgroundColor: tint }]} />
      <Text style={[chip.text, { color: colors.textSecondary }]}>
        {status ? t(`agents.status.${status}`) : t('agents.status.none')}
      </Text>
    </View>
  );
}

const chip = StyleSheet.create({
  wrap: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: spacing.sm + 2,
    paddingVertical: spacing.xs + 1,
    borderRadius: radius.pill,
  },
  dot: { width: 8, height: 8, borderRadius: 4 },
  text: type.caption,
});

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    section: {
      ...type.label,
      color: colors.textSecondary,
      paddingHorizontal: spacing.lg,
      paddingTop: spacing.sm,
      paddingBottom: spacing.xs,
      textTransform: 'uppercase',
      letterSpacing: 0.4,
    },
    row: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md + 2,
      paddingHorizontal: spacing.lg,
      paddingVertical: spacing.md,
    },
    body: { flex: 1, gap: 3 },
    name: { ...type.headline, color: colors.text },
    handle: { ...type.secondary, color: colors.textSecondary },
    grow: { flexGrow: 1 },
    padded: { paddingBottom: 96 },
  });
