import * as Clipboard from 'expo-clipboard';
import { useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { describeAgentError } from '@/agents/form';
import type { ApiKey } from '@/api/types';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useStyles, useTheme, type Theme } from '@/theme';
import { formatListTime } from '@/util/time';

// API keys: the credential a person's own systems use. Created here,
// behind their own sign-in, because a key that could make another key
// would be a second door into the account.
export default function ApiKeysScreen() {
  const router = useRouter();
  const { api } = useSession();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');
  const [minted, setMinted] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<ApiKey | null>(null);

  const load = useCallback(
    () =>
      api.listApiKeys().then(
        (list) => {
          setKeys(list);
          setLoading(false);
        },
        () => setLoading(false),
      ),
    [api],
  );

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    if (!name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const made = await api.createApiKey(name.trim());
      setMinted(made.key);
      setName('');
      setNaming(false);
      await load();
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const revoke = async () => {
    if (!confirm) return;
    setBusy(true);
    try {
      await api.revokeApiKey(confirm.id);
      setConfirm(null);
      await load();
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const copy = async () => {
    if (!minted) return;
    await Clipboard.setStringAsync(minted);
    setCopied(true);
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('keys.title')} onBack={() => router.back()} />
      <FlatList
        data={keys}
        keyExtractor={(k) => k.id}
        ListHeaderComponent={
          <View style={styles.head}>
            <Text style={styles.subtitle}>{t('keys.subtitle')}</Text>
            {minted ? (
              <View style={[styles.card, { backgroundColor: colors.surface }]}>
                <Text style={styles.cardLabel}>{t('keys.created.title')}</Text>
                <Text style={styles.key} selectable testID="new-key">
                  {minted}
                </Text>
                <Text style={styles.once}>{t('keys.created.once')}</Text>
                <View style={styles.row}>
                  <Button
                    title={copied ? t('keys.copied') : t('keys.copy')}
                    onPress={() => void copy()}
                    variant="outline"
                    icon={copied ? 'checkmark' : 'copy-outline'}
                    compact
                    testID="copy-key"
                  />
                  <Button
                    title={t('keys.done')}
                    onPress={() => {
                      setMinted(null);
                      setCopied(false);
                    }}
                    variant="plain"
                    compact
                    testID="key-done"
                  />
                </View>
                <Text style={styles.hint}>{t('keys.hint')}</Text>
              </View>
            ) : naming ? (
              <View style={styles.form}>
                <TextField
                  label={t('keys.name.label')}
                  placeholder={t('keys.name.placeholder')}
                  value={name}
                  onChangeText={(v) => {
                    setName(v);
                    setError(null);
                  }}
                  autoFocus
                  error={error}
                  returnKeyType="go"
                  onSubmitEditing={() => void create()}
                  testID="key-name"
                />
                <Button
                  title={t('keys.create')}
                  onPress={() => void create()}
                  busy={busy}
                  disabled={!name.trim()}
                  testID="create-key"
                />
                <Button title={t('common.cancel')} onPress={() => setNaming(false)} variant="plain" />
              </View>
            ) : (
              <View style={styles.form}>
                {error ? <Text style={styles.error}>{error}</Text> : null}
                <Button
                  title={t('keys.new')}
                  onPress={() => setNaming(true)}
                  variant="outline"
                  icon="add"
                  testID="new-key"
                />
              </View>
            )}
          </View>
        }
        renderItem={({ item }) => (
          <View style={styles.row2}>
            <View style={styles.body}>
              <Text style={styles.name} numberOfLines={1}>
                {item.name}
              </Text>
              <Text style={styles.meta} numberOfLines={1}>
                {item.revoked_at
                  ? t('keys.revoked')
                  : item.last_used_at
                    ? t('keys.used', { when: formatListTime(item.last_used_at) })
                    : t('keys.unused')}
              </Text>
            </View>
            {item.revoked_at ? null : (
              <Pressable
                accessibilityRole="button"
                onPress={() => setConfirm(item)}
                hitSlop={8}
                testID={`revoke-${item.id}`}
              >
                <Text style={styles.revoke}>{t('keys.revoke')}</Text>
              </Pressable>
            )}
          </View>
        )}
        refreshControl={
          <RefreshControl refreshing={loading} onRefresh={() => void load()} tintColor={colors.accent} />
        }
        ListEmptyComponent={
          loading || naming || minted ? null : (
            <EmptyState
              icon="key-outline"
              title={t('keys.empty.title')}
              subtitle={t('keys.empty.subtitle')}
            />
          )
        }
        contentContainerStyle={styles.list}
        keyboardShouldPersistTaps="handled"
      />
      <ConfirmSheet
        visible={confirm !== null}
        title={t('keys.revoke.title', { name: confirm?.name ?? '' })}
        body={t('keys.revoke.body')}
        confirmLabel={t('keys.revoke')}
        destructive
        busy={busy}
        onConfirm={() => void revoke()}
        onCancel={() => setConfirm(null)}
      />
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    list: { paddingBottom: spacing.xxl },
    head: { padding: spacing.lg, gap: spacing.md },
    subtitle: { ...type.body, color: colors.textSecondary, lineHeight: 22 },
    form: { gap: spacing.sm },
    card: { borderRadius: radius.lg, padding: spacing.lg, gap: spacing.sm },
    cardLabel: {
      ...type.label,
      color: colors.textSecondary,
      textTransform: 'uppercase',
      letterSpacing: 0.4,
    },
    key: { ...type.body, color: colors.text, fontFamily: 'monospace', lineHeight: 22 },
    once: { ...type.caption, color: colors.textSecondary },
    hint: { ...type.caption, color: colors.textSecondary, lineHeight: 18 },
    row: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.xs },
    error: { ...type.secondary, color: colors.danger },
    row2: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md,
      paddingHorizontal: spacing.lg,
      paddingVertical: spacing.md,
    },
    body: { flex: 1, gap: 3 },
    name: { ...type.headline, color: colors.text },
    meta: { ...type.secondary, color: colors.textSecondary },
    revoke: { ...type.secondary, color: colors.danger, fontWeight: '600', paddingHorizontal: spacing.sm },
  });
