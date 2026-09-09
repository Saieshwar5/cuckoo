import { useRouter } from 'expo-router';
import React from 'react';
import { Pressable, RefreshControl, SectionList, StyleSheet, Text, View } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import type { AgentStatus } from '@/api/types';
import { Avatar } from '@/components/Avatar';
import { EmptyState } from '@/components/EmptyState';
import { Fab } from '@/components/Fab';
import { Header } from '@/components/Header';
import { IconButton } from '@/components/IconButton';
import { Screen } from '@/components/Screen';
import { t } from '@/i18n';
import { agentAvatar } from '@/media/avatar';
import { radius, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';

interface Row {
  id: string;
  name: string;
  handle: string;
  status: AgentStatus | null;
  blocked: boolean;
  owned: boolean;
  hasAvatar: boolean;
}

// Agents: the ones you own and the ones you added, with their connection
// state, live. Tap one for its profile; the button makes a new one.
// Signing out lives here for now.
export default function AgentsScreen() {
  const { agents, contacts, loading, controller } = useAgents();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const create = () => router.push('/agent/new');

  const yours: Row[] = agents.map((a) => ({
    id: a.id,
    name: a.display_name,
    handle: a.handle,
    status: a.binding?.status ?? null,
    blocked: false,
    owned: true,
    hasAvatar: !!a.has_avatar,
  }));
  const added: Row[] = contacts
    .filter((c) => c.added_via !== 'owner' && !c.agent_deleted)
    .map((c) => ({
      id: c.agent.id,
      name: c.agent.display_name,
      handle: c.agent.handle,
      status: c.agent.status ?? null,
      blocked: c.blocked,
      owned: false,
      hasAvatar: !!c.agent.has_avatar,
    }));
  const sections = [
    ...(yours.length ? [{ title: t('agents.yours'), data: yours }] : []),
    ...(added.length ? [{ title: t('agents.added'), data: added }] : []),
  ];

  return (
    <Screen padded={false}>
      <Header
        title={t('agents.title')}
        actions={
          <>
            {/* What Cuckoo runs. The one way in that needs nobody to have
                given you a code and nothing of your own to run. */}
            <IconButton
              icon="sparkles-outline"
              label={t('catalogue.title')}
              onPress={() => router.push('/catalogue')}
              testID="open-catalogue"
            />
            <IconButton
              icon="person-circle-outline"
              label={t('profile.open')}
              onPress={() => router.push('/me')}
              testID="open-profile"
            />
            <IconButton
              icon="settings-outline"
              label={t('settings.title')}
              onPress={() => router.push('/settings')}
              testID="open-settings"
            />
          </>
        }
      />
      <SectionList
        sections={sections}
        keyExtractor={(row) => row.id}
        renderSectionHeader={({ section }) => <Text style={styles.section}>{section.title}</Text>}
        renderItem={({ item }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push({ pathname: '/agent/[id]', params: { id: item.id } })}
            style={({ pressed }) => [styles.row, pressed && { backgroundColor: colors.surface }]}
            testID={`agent-row-${item.id}`}
          >
            <Avatar name={item.name} status={item.status} source={agentAvatar(item.id, item.hasAvatar)} />
            <View style={styles.body}>
              <Text style={styles.name} numberOfLines={1}>
                {item.name}
              </Text>
              <Text style={styles.handle} numberOfLines={1}>
                @{item.handle}
              </Text>
            </View>
            <StatusChip status={item.status} blocked={item.blocked} colors={colors} />
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
              action={{ title: t('agents.empty.browse'), onPress: () => router.push('/catalogue') }}
              secondary={{ title: t('agents.empty.action'), onPress: create }}
            />
          )
        }
        contentContainerStyle={sections.length === 0 ? styles.grow : styles.padded}
        stickySectionHeadersEnabled={false}
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

// StatusChip says in a word what the avatar's dot says in a colour, or
// that the person has blocked this one.
function StatusChip({
  status,
  blocked,
  colors,
}: {
  status: AgentStatus | null;
  blocked: boolean;
  colors: Palette;
}) {
  const tint = blocked ? colors.danger : statusColor(colors, status);
  const label = blocked
    ? t('agent.blocked')
    : status
      ? t(`agents.status.${status}`)
      : t('agents.status.none');
  return (
    <View style={[chip.wrap, { backgroundColor: colors.surface }]}>
      <View style={[chip.dot, { backgroundColor: tint }]} />
      <Text style={[chip.text, { color: colors.textSecondary }]}>{label}</Text>
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
      paddingTop: spacing.md,
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
