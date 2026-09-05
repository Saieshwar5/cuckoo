import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useAgent, useAgents } from '@/agents/AgentsProvider';
import type { AgentStatus } from '@/api/types';
import { useChats } from '@/chats/useChats';
import { Avatar } from '@/components/Avatar';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { radius, sizes, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';

// The agent's profile: one screen for every agent, owned or added. Today
// only owned ones exist, so the owner's actions are all there is.
export default function AgentProfileScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const agent = useAgent(id);
  const { loading, controller } = useAgents();
  const { conversations } = useChats();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/agents'));
  const dm = conversations.find((c) => c.participants.some((p) => p.kind === 'agent' && p.id === id));

  const remove = async () => {
    if (!controller || !agent) return;
    setDeleting(true);
    try {
      await controller.remove(agent.id);
      setConfirmDelete(false);
      router.replace('/(tabs)/agents');
    } catch {
      setDeleting(false);
    }
  };

  if (!agent) {
    return (
      <Screen padded={false}>
        <TopBar title="" onBack={back} />
        {loading ? null : <EmptyState icon="help-circle-outline" title={t('agent.notfound')} subtitle="" />}
      </Screen>
    );
  }

  const status = agent.binding?.status ?? null;
  const connected = status === 'connected';
  return (
    <Screen padded={false}>
      <TopBar
        title=""
        onBack={back}
        action={
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push({ pathname: '/agent/[id]/edit', params: { id } })}
            hitSlop={8}
            testID="agent-edit"
          >
            <Text style={styles.editLink}>{t('agent.edit')}</Text>
          </Pressable>
        }
      />
      <ScrollView contentContainerStyle={styles.body}>
        <View style={styles.head}>
          <Avatar name={agent.display_name} size={sizes.avatarLarge} status={status} />
          <Text style={styles.name}>{agent.display_name}</Text>
          <Text style={styles.handle}>@{agent.handle}</Text>
          <View style={styles.ownerRow}>
            <Text style={styles.owner}>{t('agent.by.you')}</Text>
            <View style={[styles.badge, { backgroundColor: colors.surface }]}>
              <Text style={[styles.badgeText, { color: colors.textSecondary }]}>{t('agent.unverified')}</Text>
            </View>
          </View>
          {agent.description ? <Text style={styles.description}>{agent.description}</Text> : null}
        </View>

        <View style={styles.actions}>
          <ActionTile
            icon="chatbubble-outline"
            label={t('agent.message')}
            onPress={() => dm && router.push({ pathname: '/chat/[id]', params: { id: dm.id } })}
            disabled={!dm}
            colors={colors}
            testID="agent-message"
          />
          <ActionTile
            icon={connected ? 'link-outline' : 'flash-outline'}
            label={t('agent.connect')}
            onPress={() => router.push({ pathname: '/agent/[id]/connect', params: { id } })}
            colors={colors}
            testID="agent-connect"
          />
        </View>

        <View style={[styles.card, { backgroundColor: colors.surface }]}>
          <Text style={styles.cardLabel}>{t('agent.status.title')}</Text>
          <View style={styles.statusRow}>
            <View style={[styles.dot, { backgroundColor: statusColor(colors, status) }]} />
            <Text style={styles.statusText} testID="agent-status">
              {status ? t(`agents.status.${status}`) : t('agents.status.none')}
            </Text>
          </View>
          {!connected ? <Text style={styles.statusHint}>{t('agent.notconnected')}</Text> : null}
        </View>

        <View style={styles.footer}>
          <Button
            title={t('agent.delete')}
            onPress={() => setConfirmDelete(true)}
            variant="plain"
            tone="danger"
            testID="agent-delete"
          />
        </View>
      </ScrollView>
      <ConfirmSheet
        visible={confirmDelete}
        title={t('agent.delete.title', { name: agent.display_name })}
        body={t('agent.delete.body')}
        confirmLabel={t('agent.delete')}
        destructive
        busy={deleting}
        onConfirm={() => void remove()}
        onCancel={() => setConfirmDelete(false)}
      />
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

// ActionTile is one of the pill cards under the name, as the reference
// apps draw a contact's actions.
function ActionTile({
  icon,
  label,
  onPress,
  disabled,
  colors,
  testID,
}: {
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  onPress: () => void;
  disabled?: boolean;
  colors: Palette;
  testID: string;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled }}
      disabled={disabled}
      onPress={onPress}
      testID={testID}
      style={({ pressed }) => [
        tile.base,
        { backgroundColor: pressed ? colors.surfaceStrong : colors.surface },
        disabled && tile.disabled,
      ]}
    >
      <Ionicons name={icon} size={22} color={colors.accent} />
      <Text style={[tile.label, { color: colors.text }]}>{label}</Text>
    </Pressable>
  );
}

const tile = StyleSheet.create({
  base: {
    flex: 1,
    alignItems: 'center',
    gap: spacing.xs,
    paddingVertical: spacing.md,
    borderRadius: radius.lg,
  },
  label: type.label,
  disabled: { opacity: 0.5 },
});

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl, gap: spacing.lg },
    head: { alignItems: 'center', gap: spacing.xs },
    name: { ...type.title, color: colors.text, marginTop: spacing.sm },
    handle: { ...type.secondary, color: colors.textSecondary },
    ownerRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, marginTop: spacing.xs },
    owner: { ...type.secondary, color: colors.textSecondary },
    badge: { paddingHorizontal: spacing.sm, paddingVertical: 2, borderRadius: radius.pill },
    badgeText: type.caption,
    description: {
      ...type.body,
      color: colors.text,
      textAlign: 'center',
      marginTop: spacing.md,
      lineHeight: 22,
    },
    editLink: { ...type.body, color: colors.accent, fontWeight: '600', paddingHorizontal: spacing.md },
    actions: { flexDirection: 'row', gap: spacing.sm },
    card: { borderRadius: radius.lg, padding: spacing.lg, gap: spacing.sm },
    cardLabel: { ...type.label, color: colors.textSecondary, textTransform: 'uppercase', letterSpacing: 0.4 },
    statusRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    dot: { width: 10, height: 10, borderRadius: 5 },
    statusText: { ...type.headline, color: colors.text },
    statusHint: { ...type.secondary, color: colors.textSecondary, lineHeight: 20 },
    footer: { alignItems: 'center', marginTop: spacing.md },
  });
