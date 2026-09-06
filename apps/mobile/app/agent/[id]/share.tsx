import * as Clipboard from 'expo-clipboard';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { Image, ScrollView, Share, StyleSheet, Text, View } from 'react-native';

import { useAgent } from '@/agents/AgentsProvider';
import { describeAgentError } from '@/agents/form';
import { clearShareCode, loadShareCode, saveShareCode } from '@/agents/shareCode';
import type { PairToken } from '@/api/types';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { hubUrl } from '@/config';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useStyles, useTheme, type Theme } from '@/theme';

// Share: the agent's code, as a QR picture and a link. One static code per
// agent, kept on this phone since the hub shows it once; Stop sharing
// withdraws it and everyone who already added the agent keeps it.
export default function ShareScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { api } = useSession();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const agent = useAgent(id);
  const [minted, setMinted] = useState<{ tokenId: string; code: string; url: string; qr: string } | null>(
    null,
  );
  const [token, setToken] = useState<PairToken | null>(null);
  const [checked, setChecked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [confirmStop, setConfirmStop] = useState(false);

  // A code kept from an earlier visit is still good if the hub still lists
  // it as live; otherwise it is forgotten and a new one can be made.
  useEffect(() => {
    let cancelled = false;
    Promise.all([loadShareCode(id), api.listPairTokens(id)]).then(
      ([kept, tokens]) => {
        if (cancelled) return;
        const live = kept ? tokens.find((tk) => tk.id === kept.tokenId && !tk.revoked_at) : undefined;
        if (kept && live) {
          setToken(live);
          api.pairTokenQR(id, kept.tokenId, kept.code).then(
            (pic) => {
              if (!cancelled)
                setMinted({ tokenId: kept.tokenId, code: kept.code, url: pic.url, qr: pic.qr_png });
            },
            () => {
              if (!cancelled) setMinted({ tokenId: kept.tokenId, code: kept.code, url: '', qr: '' });
            },
          );
        } else if (kept) {
          void clearShareCode(id);
        }
        setChecked(true);
      },
      () => {
        if (!cancelled) setChecked(true);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [api, id]);

  const create = async () => {
    setBusy(true);
    setError(null);
    try {
      const m = await api.createPairToken(id, {});
      await saveShareCode(id, { tokenId: m.token.id, code: m.code });
      setMinted({ tokenId: m.token.id, code: m.code, url: m.url, qr: m.qr_png });
      setToken(m.token);
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const stop = async () => {
    if (!minted) return;
    setBusy(true);
    try {
      await api.revokePairToken(id, minted.tokenId);
      await clearShareCode(id);
      setMinted(null);
      setToken(null);
      setConfirmStop(false);
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const link = minted ? minted.url || `${hubUrl}/p/${minted.code}` : '';
  const copy = async () => {
    await Clipboard.setStringAsync(link);
    setCopied(true);
  };
  const share = () => Share.share({ message: link, url: link }).catch(() => undefined);

  return (
    <Screen padded={false}>
      <TopBar title={t('share.title')} onBack={() => router.back()} />
      <ScrollView contentContainerStyle={styles.body}>
        {agent ? (
          <Text style={styles.subtitle}>{t('share.subtitle', { name: agent.display_name })}</Text>
        ) : null}
        {minted ? (
          <>
            {minted.qr ? (
              <View style={[styles.card, { backgroundColor: '#FFFFFF' }]}>
                <Image
                  source={{ uri: minted.qr }}
                  style={styles.qr}
                  accessibilityLabel={t('share.title')}
                  testID="qr"
                />
              </View>
            ) : null}
            <View style={[styles.card, { backgroundColor: colors.surface }]}>
              <Text style={styles.cardLabel}>{t('share.link')}</Text>
              <Text style={styles.link} selectable testID="share-link">
                {link}
              </Text>
              <View style={styles.row}>
                <Button
                  title={copied ? t('share.copied') : t('share.copy')}
                  onPress={() => void copy()}
                  variant="outline"
                  icon={copied ? 'checkmark' : 'copy-outline'}
                  compact
                  testID="copy-link"
                />
                <Button
                  title={t('agent.share')}
                  onPress={share}
                  variant="outline"
                  icon="share-outline"
                  compact
                />
              </View>
            </View>
            <Text style={styles.uses} testID="share-uses">
              {uses(token?.use_count ?? 0)}
            </Text>
            <Button
              title={t('share.stop')}
              onPress={() => setConfirmStop(true)}
              variant="plain"
              tone="danger"
              testID="stop-sharing"
            />
          </>
        ) : checked ? (
          <>
            <Text style={styles.hint}>{t('share.hint')}</Text>
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <Button
              title={busy ? t('share.creating') : t('share.create')}
              onPress={() => void create()}
              busy={busy}
              testID="create-code"
            />
          </>
        ) : null}
      </ScrollView>
      <ConfirmSheet
        visible={confirmStop}
        title={t('share.stop.title')}
        body={t('share.stop.body')}
        confirmLabel={t('share.stop')}
        destructive
        busy={busy}
        onConfirm={() => void stop()}
        onCancel={() => setConfirmStop(false)}
      />
    </Screen>
  );
}

function uses(n: number): string {
  if (n === 0) return t('share.uses.none');
  if (n === 1) return t('share.uses.one');
  return t('share.uses', { count: n });
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl, gap: spacing.md },
    subtitle: { ...type.body, color: colors.textSecondary, lineHeight: 22 },
    card: { borderRadius: radius.lg, padding: spacing.lg, gap: spacing.sm, alignItems: 'center' },
    cardLabel: {
      ...type.label,
      color: colors.textSecondary,
      textTransform: 'uppercase',
      letterSpacing: 0.4,
      alignSelf: 'flex-start',
    },
    qr: { width: 240, height: 240 },
    link: { ...type.secondary, color: colors.text, alignSelf: 'flex-start' },
    row: {
      flexDirection: 'row',
      flexWrap: 'wrap',
      gap: spacing.sm,
      alignSelf: 'flex-start',
      marginTop: spacing.xs,
    },
    uses: { ...type.secondary, color: colors.textSecondary, textAlign: 'center' },
    hint: { ...type.body, color: colors.textSecondary, lineHeight: 22 },
    error: { ...type.secondary, color: colors.danger },
  });
