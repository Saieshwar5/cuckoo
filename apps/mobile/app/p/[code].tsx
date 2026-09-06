import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import { useChats } from '@/chats/useChats';
import type { PairResolve } from '@/api/types';
import { ApiError } from '@/api/client';
import { Avatar } from '@/components/Avatar';
import { agentAvatar } from '@/media/avatar';
import { Button } from '@/components/Button';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, sizes, spacing, type, useStyles, useTheme, type Theme } from '@/theme';

// The landing for a scanned code or an opened link: the agent's card and
// one button. Scanning is not adding; tapping Add is.
export default function PairScreen() {
  const { code } = useLocalSearchParams<{ code: string }>();
  const router = useRouter();
  const { api } = useSession();
  const { controller } = useAgents();
  const { refresh: refreshChats } = useChats();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [card, setCard] = useState<PairResolve | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api.resolvePair(code).then(
      (c) => {
        if (!cancelled) setCard(c);
      },
      (err) => {
        if (!cancelled) setError(describePairError(err));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, code]);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/chats'));

  const add = async () => {
    if (!controller) return;
    setBusy(true);
    try {
      const accepted = await controller.accept(code);
      await refreshChats();
      router.replace({ pathname: '/chat/[id]', params: { id: accepted.conversation.id } });
    } catch (err) {
      setError(describePairError(err));
      setBusy(false);
    }
  };

  const open = () => {
    if (card?.conversation_id)
      router.replace({ pathname: '/chat/[id]', params: { id: card.conversation_id } });
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('pair.title')} onBack={back} />
      {error ? (
        <EmptyState
          icon="link-outline"
          title={error}
          subtitle=""
          action={{ title: t('common.back'), onPress: back }}
        />
      ) : !card ? (
        <ActivityIndicator color={colors.accent} style={styles.center} />
      ) : (
        <View style={styles.body}>
          <Avatar
            name={card.agent.display_name}
            size={sizes.avatarLarge}
            status={card.agent.status ?? null}
            source={agentAvatar(card.agent.id, card.agent.has_avatar)}
          />
          <Text style={styles.name} testID="pair-name">
            {card.agent.display_name}
          </Text>
          <Text style={styles.handle}>@{card.agent.handle}</Text>
          <View style={styles.ownerRow}>
            <Text style={styles.owner}>{t('agent.by.someone', { name: card.agent.owner.display_name })}</Text>
            <View style={[styles.badge, { backgroundColor: colors.surface }]}>
              <Text style={[styles.badgeText, { color: colors.textSecondary }]}>{t('agent.unverified')}</Text>
            </View>
          </View>
          {card.agent.description ? <Text style={styles.description}>{card.agent.description}</Text> : null}
          <View style={styles.actions}>
            {card.already_added && !card.blocked ? (
              <>
                <Text style={styles.note}>{t('pair.added')}</Text>
                <Button title={t('pair.open')} onPress={open} testID="pair-open" />
              </>
            ) : (
              <Button
                title={card.blocked ? t('pair.unblock') : t('pair.add')}
                onPress={() => void add()}
                busy={busy}
                testID="pair-add"
              />
            )}
            <Button title={t('common.cancel')} onPress={back} variant="plain" />
          </View>
        </View>
      )}
    </Screen>
  );
}

function describePairError(err: unknown): string {
  if (err instanceof ApiError) {
    const known = t(`pair.error.${err.code}`);
    return known !== `pair.error.${err.code}` ? known : err.message;
  }
  if (err instanceof Error && err.name === 'NetworkError') return t('common.error.network');
  return t('common.error.unknown');
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    center: { flex: 1 },
    body: { flex: 1, alignItems: 'center', padding: spacing.xl, paddingTop: spacing.xxl, gap: spacing.xs },
    name: { ...type.title, color: colors.text, marginTop: spacing.md },
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
    actions: { alignSelf: 'stretch', marginTop: spacing.xl, gap: spacing.sm },
    note: { ...type.secondary, color: colors.textSecondary, textAlign: 'center' },
  });
