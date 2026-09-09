import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { ActivityIndicator, FlatList, StyleSheet, Text, View } from 'react-native';

import { ApiError } from '@/api/client';
import type { CatalogueEntry } from '@/api/types';
import { Avatar } from '@/components/Avatar';
import { Button } from '@/components/Button';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { useChats } from '@/chats/useChats';
import { t } from '@/i18n';
import { agentAvatar } from '@/media/avatar';
import { useSession } from '@/session/SessionProvider';
import { sizes, spacing, type, useStyles, useTheme, type Theme } from '@/theme';

// The agents Cuckoo runs, and one tap to have one.
//
// A row is the same card a scanned code resolves to, drawn smaller: a person
// meets an agent described the same way whether they found it in this list or
// on a poster. Nothing here needs a code — being listed is the invitation.
export default function CatalogueScreen() {
  const router = useRouter();
  const { api } = useSession();
  const { refresh: refreshChats } = useChats();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [agents, setAgents] = useState<CatalogueEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState<string | null>(null);

  // A counter rather than a callback, so asking again is a dependency change
  // and the fetch stays inside the effect, where an unmounted screen can be
  // ignored rather than written to.
  const [attempt, setAttempt] = useState(0);
  const retry = () => setAttempt((n) => n + 1);

  useEffect(() => {
    let cancelled = false;
    api.listCatalogue().then(
      (list) => {
        if (!cancelled) setAgents(list);
      },
      (err: unknown) => {
        if (!cancelled) setError(describeError(err));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, attempt]);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/agents'));

  const open = (conversationId: string) =>
    router.push({ pathname: '/chat/[id]', params: { id: conversationId } });

  const add = async (entry: CatalogueEntry) => {
    setAdding(entry.agent.id);
    try {
      const accepted = await api.addFromCatalogue(entry.agent.id);
      await refreshChats();
      router.push({ pathname: '/chat/[id]', params: { id: accepted.conversation.id } });
    } catch (err) {
      setError(describeError(err));
    } finally {
      setAdding(null);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('catalogue.title')} onBack={back} />
      {error && !agents ? (
        <EmptyState
          icon="cloud-offline-outline"
          title={error}
          subtitle=""
          action={{ title: t('common.retry'), onPress: retry }}
        />
      ) : !agents ? (
        <ActivityIndicator color={colors.accent} style={styles.center} />
      ) : agents.length === 0 ? (
        <EmptyState
          icon="sparkles-outline"
          title={t('catalogue.empty.title')}
          subtitle={t('catalogue.empty.subtitle')}
        />
      ) : (
        <FlatList
          data={agents}
          keyExtractor={(entry) => entry.agent.id}
          contentContainerStyle={styles.list}
          ListHeaderComponent={<Text style={styles.intro}>{t('catalogue.intro')}</Text>}
          renderItem={({ item }) => (
            <View style={styles.row} testID={`catalogue-row-${item.agent.handle}`}>
              <Avatar
                name={item.agent.display_name}
                size={sizes.avatar}
                status={item.agent.status ?? null}
                source={agentAvatar(item.agent.id, item.agent.has_avatar)}
              />
              <View style={styles.details}>
                <Text style={styles.name} numberOfLines={1}>
                  {item.agent.display_name}
                </Text>
                {item.agent.description ? (
                  <Text style={styles.description} numberOfLines={2}>
                    {item.agent.description}
                  </Text>
                ) : null}
              </View>
              {/* Added already: the button opens the chat rather than
                  offering a thing they have. */}
              {item.already_added && item.conversation_id ? (
                <Button
                  title={t('catalogue.open')}
                  onPress={() => open(item.conversation_id as string)}
                  variant="plain"
                  testID={`catalogue-open-${item.agent.handle}`}
                />
              ) : (
                <Button
                  title={t('catalogue.add')}
                  onPress={() => void add(item)}
                  busy={adding === item.agent.id}
                  testID={`catalogue-add-${item.agent.handle}`}
                />
              )}
            </View>
          )}
        />
      )}
    </Screen>
  );
}

function describeError(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error && err.name === 'NetworkError') return t('common.error.network');
  return t('common.error.unknown');
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    center: { flex: 1 },
    list: { padding: spacing.lg, gap: spacing.lg },
    intro: { ...type.secondary, color: colors.textSecondary, marginBottom: spacing.xs },
    row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
    details: { flex: 1, gap: 2 },
    name: { ...type.headline, color: colors.text },
    description: { ...type.secondary, color: colors.textSecondary },
  });
