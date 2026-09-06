import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useAgent, useAgents, useContact } from '@/agents/AgentsProvider';
import { useChats } from '@/chats/useChats';
import { isMuted } from '@/agents/store';
import type { AgentStatus } from '@/api/types';
import { ActionSheet, type SheetAction } from '@/components/ActionSheet';
import { Avatar } from '@/components/Avatar';
import { agentAvatar } from '@/media/avatar';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { radius, sizes, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';
import { formatClock, formatDay } from '@/util/time';

// A mute with no end is a date nobody will reach.
const ALWAYS = '2200-01-01T00:00:00.000Z';
const HOUR = 60 * 60 * 1000;

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
  const chats = useChats();
  const [confirm, setConfirm] = useState<'delete' | 'block' | 'remove' | null>(null);
  const [sheet, setSheet] = useState<'more' | 'mute' | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
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
  // decide changes one of the person's settings for this agent. The row
  // already reflects it; a refusal — a fourth pin — is said in a line.
  const decide = async (change: Parameters<NonNullable<typeof controller>['settings']>[1]) => {
    if (!controller) return;
    setNotice(null);
    try {
      await controller.settings(id, change);
    } catch (err) {
      const code = (err as { code?: string }).code;
      setNotice(code === 'too_many_pins' ? t('agent.pin.full') : ((err as Error).message ?? ''));
    }
  };
  // removeFromList takes the agent out of the list. The chat list is the
  // hub's and the hub no longer lists this chat, so it is asked again
  // rather than second-guessed here.
  const removeFromList = async () => {
    if (!controller) return;
    setBusy(true);
    try {
      await controller.removeContact(id);
      await chats.refresh();
      setConfirm(null);
      router.replace('/(tabs)/chats');
    } catch {
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
        hasAvatar: !!owned.has_avatar,
      }
    : contact
      ? {
          name: contact.agent.display_name,
          handle: contact.agent.handle,
          description: contact.agent.description,
          status: contact.agent.status ?? null,
          owner: t('agent.by.someone', { name: contact.agent.owner.display_name }),
          hasAvatar: !!contact.agent.has_avatar,
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
  const muted = contact ? isMuted(contact) : false;
  const pinned = contact?.pinned ?? false;
  const archived = contact?.archived ?? false;
  const mutedLabel = !contact?.muted_until
    ? null
    : contact.muted_until >= ALWAYS
      ? t('agent.muted.always')
      : t('agent.muted.until', {
          when: `${formatDay(contact.muted_until, new Date(), { today: t('chat.day.today'), yesterday: t('chat.day.yesterday') })} ${formatClock(contact.muted_until)}`,
        });
  const more: SheetAction[] = [
    muted
      ? {
          icon: 'volume-high-outline',
          label: t('agent.unmute'),
          onPress: () => void decide({ muted_until: null }),
          testID: 'agent-unmute',
        }
      : {
          icon: 'volume-mute-outline',
          label: t('agent.mute'),
          onPress: () => setSheet('mute'),
          testID: 'agent-mute',
        },
    pinned
      ? {
          icon: 'pin-outline',
          label: t('agent.unpin'),
          onPress: () => void decide({ pinned: false }),
          testID: 'agent-unpin',
        }
      : {
          icon: 'pin',
          label: t('agent.pin'),
          onPress: () => void decide({ pinned: true }),
          testID: 'agent-pin',
        },
    archived
      ? {
          icon: 'arrow-undo-outline',
          label: t('agent.unarchive'),
          onPress: () => void decide({ archived: false }),
          testID: 'agent-unarchive',
        }
      : {
          icon: 'archive-outline',
          label: t('agent.archive'),
          onPress: () => void decide({ archived: true }),
          testID: 'agent-archive',
        },
    ...(contact && !owned
      ? [
          {
            icon: 'person-remove-outline' as const,
            label: t('agent.remove'),
            onPress: () => setConfirm('remove'),
            testID: 'agent-remove',
          },
        ]
      : []),
  ];
  const muteFor = (ms: number | null) =>
    void decide({ muted_until: ms === null ? ALWAYS : new Date(Date.now() + ms).toISOString() });
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
          <Avatar
            name={view.name}
            size={sizes.avatarLarge}
            status={view.status}
            source={agentAvatar(id, view.hasAvatar)}
          />
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
          {muted || pinned ? (
            <View style={styles.ownerRow}>
              {muted && mutedLabel ? (
                <View
                  style={[styles.badge, styles.badgeRow, { backgroundColor: colors.surface }]}
                  testID="badge-muted"
                >
                  <Ionicons name="volume-mute-outline" size={13} color={colors.textSecondary} />
                  <Text style={[styles.badgeText, { color: colors.textSecondary }]}>{mutedLabel}</Text>
                </View>
              ) : null}
              {pinned ? (
                <View
                  style={[styles.badge, styles.badgeRow, { backgroundColor: colors.surface }]}
                  testID="badge-pinned"
                >
                  <Ionicons name="pin" size={13} color={colors.textSecondary} />
                  <Text style={[styles.badgeText, { color: colors.textSecondary }]}>{t('agent.pin')}</Text>
                </View>
              ) : null}
            </View>
          ) : null}
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
          {contact ? (
            <ActionTile
              icon="ellipsis-horizontal"
              label={t('agent.more')}
              onPress={() => setSheet('more')}
              colors={colors}
              testID="agent-more"
            />
          ) : null}
        </View>
        {notice ? (
          <Text style={styles.notice} testID="agent-notice">
            {notice}
          </Text>
        ) : null}

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
      <ActionSheet visible={sheet === 'more'} onClose={() => setSheet(null)} actions={more} />
      <ActionSheet
        visible={sheet === 'mute'}
        onClose={() => setSheet(null)}
        actions={[
          {
            icon: 'time-outline',
            label: t('agent.mute.8h'),
            onPress: () => muteFor(8 * HOUR),
            testID: 'mute-8h',
          },
          {
            icon: 'calendar-outline',
            label: t('agent.mute.1w'),
            onPress: () => muteFor(7 * 24 * HOUR),
            testID: 'mute-1w',
          },
          {
            icon: 'infinite-outline',
            label: t('agent.mute.always'),
            onPress: () => muteFor(null),
            testID: 'mute-always',
          },
        ]}
      />
      <ConfirmSheet
        visible={confirm === 'remove'}
        title={t('agent.remove.title', { name: view.name })}
        body={t('agent.remove.body')}
        confirmLabel={t('agent.remove')}
        busy={busy}
        onConfirm={() => void removeFromList()}
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
    badgeRow: { flexDirection: 'row', alignItems: 'center', gap: 4 },
    badgeText: type.caption,
    notice: { ...type.secondary, color: colors.danger, textAlign: 'center', marginTop: -spacing.sm },
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
