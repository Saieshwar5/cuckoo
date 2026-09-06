import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useAgent, useAgents, useContact } from '@/agents/AgentsProvider';
import type { AgentStatus } from '@/api/types';
import { Avatar } from '@/components/Avatar';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { radius, sizes, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';

// The agent's profile: one screen for every agent, owned or added. An
// owner sees the levers; someone who added it sees who is behind it and
// the way out.
export default function AgentProfileScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const owned = useAgent(id);
  const contact = useContact(id);
  const { loading, controller } = useAgents();
  const [confirm, setConfirm] = useState<'delete' | 'block' | null>(null);
  const [busy, setBusy] = useState(false);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/agents'));
  const go = (pathname: '/agent/[id]/edit' | '/agent/[id]/connect' | '/agent/[id]/share') =>
    router.push({ pathname, params: { id } });

  const remove = async () => {
    if (!controller || !owned) return;
    setBusy(true);
    try {
      await controller.remove(owned.id);
      setConfirm(null);
      router.replace('/(tabs)/agents');
    } catch {
      setBusy(false);
    }
  };
  const block = async () => {
    if (!controller) return;
    setBusy(true);
    try {
      await controller.block(id);
      setConfirm(null);
    } finally {
      setBusy(false);
    }
  };
  const unblock = async () => {
    if (!controller) return;
    setBusy(true);
    try {
      await controller.unblock(id);
    } finally {
      setBusy(false);
    }
  };

  const view = owned
    ? {
        name: owned.display_name,
        handle: owned.handle,
        description: owned.description,
        status: owned.binding?.status ?? null,
        owner: t('agent.by.you'),
      }
    : contact
      ? {
          name: contact.agent.display_name,
          handle: contact.agent.handle,
          description: contact.agent.description,
          status: contact.agent.status ?? null,
          owner: t('agent.by.someone', { name: contact.agent.owner.display_name }),
        }
      : null;

  if (!view) {
    return (
      <Screen padded={false}>
        <TopBar title="" onBack={back} />
        {loading ? null : <EmptyState icon="help-circle-outline" title={t('agent.notfound')} subtitle="" />}
      </Screen>
    );
  }

  const connected = view.status === 'connected';
  const blocked = contact?.blocked ?? false;
  return (
    <Screen padded={false}>
      <TopBar
        title=""
        onBack={back}
        action={
          owned ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => go('/agent/[id]/edit')}
              hitSlop={8}
              testID="agent-edit"
            >
              <Text style={styles.editLink}>{t('agent.edit')}</Text>
            </Pressable>
          ) : undefined
        }
      />
      <ScrollView contentContainerStyle={styles.body}>
        <View style={styles.head}>
          <Avatar name={view.name} size={sizes.avatarLarge} status={view.status} />
          <Text style={styles.name}>{view.name}</Text>
          <Text style={styles.handle}>@{view.handle}</Text>
          <View style={styles.ownerRow}>
            <Text style={styles.owner}>{view.owner}</Text>
            <View style={[styles.badge, { backgroundColor: colors.surface }]}>
              <Text style={[styles.badgeText, { color: colors.textSecondary }]}>{t('agent.unverified')}</Text>
            </View>
            {blocked ? (
              <View style={[styles.badge, { backgroundColor: colors.danger }]}>
                <Text style={[styles.badgeText, { color: '#FFFFFF' }]}>{t('agent.blocked')}</Text>
              </View>
            ) : null}
          </View>
          {view.description ? <Text style={styles.description}>{view.description}</Text> : null}
        </View>

        <View style={styles.actions}>
          <ActionTile
            icon="chatbubble-outline"
            label={t('agent.message')}
            onPress={() =>
              contact && router.push({ pathname: '/chat/[id]', params: { id: contact.conversation_id } })
            }
            disabled={!contact}
            colors={colors}
            testID="agent-message"
          />
          {owned ? (
            <>
              <ActionTile
                icon={connected ? 'link-outline' : 'flash-outline'}
                label={t('agent.connect')}
                onPress={() => go('/agent/[id]/connect')}
                colors={colors}
                testID="agent-connect"
              />
              <ActionTile
                icon="qr-code-outline"
                label={t('agent.share')}
                onPress={() => go('/agent/[id]/share')}
                colors={colors}
                testID="agent-share"
              />
            </>
          ) : (
            <ActionTile
              icon={blocked ? 'lock-open-outline' : 'ban-outline'}
              label={blocked ? t('agent.unblock') : t('agent.block')}
              onPress={() => (blocked ? void unblock() : setConfirm('block'))}
              colors={colors}
              testID={blocked ? 'agent-unblock' : 'agent-block'}
            />
          )}
        </View>

        <View style={[styles.card, { backgroundColor: colors.surface }]}>
          <Text style={styles.cardLabel}>{t('agent.status.title')}</Text>
          <View style={styles.statusRow}>
            <View style={[styles.dot, { backgroundColor: statusColor(colors, view.status) }]} />
            <Text style={styles.statusText} testID="agent-status">
              {view.status ? t(`agents.status.${view.status}`) : t('agents.status.none')}
            </Text>
          </View>
          {owned && !connected ? <Text style={styles.statusHint}>{t('agent.notconnected')}</Text> : null}
        </View>

        {owned ? (
          <View style={styles.footer}>
            <Button
              title={t('agent.delete')}
              onPress={() => setConfirm('delete')}
              variant="plain"
              tone="danger"
              testID="agent-delete"
            />
          </View>
        ) : null}
      </ScrollView>
      <ConfirmSheet
        visible={confirm === 'delete'}
        title={t('agent.delete.title', { name: view.name })}
        body={t('agent.delete.body')}
        confirmLabel={t('agent.delete')}
        destructive
        busy={busy}
        onConfirm={() => void remove()}
        onCancel={() => setConfirm(null)}
      />
      <ConfirmSheet
        visible={confirm === 'block'}
        title={t('agent.block.title', { name: view.name })}
        body={t('agent.block.body')}
        confirmLabel={t('agent.block')}
        destructive
        busy={busy}
        onConfirm={() => void block()}
        onCancel={() => setConfirm(null)}
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
