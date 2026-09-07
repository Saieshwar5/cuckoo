import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import type { StorageUsage } from '@/api/types';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { storageLine } from '@/media/format';
import { forgetMedia } from '@/media/source';
import { useSession } from '@/session/SessionProvider';
import {
  radius,
  spacing,
  type,
  usePreference,
  useStyles,
  useTheme,
  type Preference,
  type Theme,
} from '@/theme';
import { openLink } from '@/util/open';

const VERSION = '0.1.0';
const SOURCE = 'https://github.com/Saieshwar5/cuckoo';
const LOOKS: Preference[] = ['system', 'light', 'dark'];

// Settings: the few things about the app itself that are the person's to
// decide. Language arrives with the next step, when agents are told it.
export default function SettingsScreen() {
  const router = useRouter();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const { preference, setPreference } = usePreference();
  const { api, signOut } = useSession();
  const [cleared, setCleared] = useState(false);
  const [confirm, setConfirm] = useState<'delete' | null>(null);
  const [busy, setBusy] = useState(false);
  // What the hub holds for this person, asked once when the screen opens.
  // Until it answers, and if it does not, the line is simply not there:
  // a spinner or an apology would make more of it than it is.
  const [storage, setStorage] = useState<StorageUsage | null>(null);
  useEffect(() => {
    let cancelled = false;
    api.storage().then(
      (usage) => {
        if (!cancelled) setStorage(usage);
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  }, [api]);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/agents'));
  const clearMedia = () => {
    forgetMedia();
    setCleared(true);
  };
  // Closing the account: the hub retires the person's agents and closes
  // every session; this device then forgets the one it had.
  const deleteAccount = async () => {
    setBusy(true);
    try {
      await api.deleteMe();
      await signOut();
    } catch {
      setBusy(false);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('settings.title')} onBack={back} />
      <ScrollView contentContainerStyle={styles.body}>
        <Text style={styles.section}>{t('settings.appearance')}</Text>
        <View style={styles.pills}>
          {LOOKS.map((look) => {
            const on = preference === look;
            return (
              <Pressable
                key={look}
                accessibilityRole="button"
                accessibilityState={{ selected: on }}
                onPress={() => setPreference(look)}
                testID={`look-${look}`}
                style={[styles.pill, { backgroundColor: on ? colors.accent : colors.surface }]}
              >
                <Text style={[styles.pillText, { color: on ? colors.onAccent : colors.text }]}>
                  {t(`settings.appearance.${look}`)}
                </Text>
              </Pressable>
            );
          })}
        </View>

        <Text style={styles.section}>{t('settings.account')}</Text>
        <View style={[styles.card, { backgroundColor: colors.surface }]}>
          <Row
            icon="person-circle-outline"
            label={t('settings.profile')}
            onPress={() => router.push('/me')}
            testID="settings-profile"
          />
          <Row
            icon="key-outline"
            label={t('settings.keys')}
            onPress={() => router.push('/api-keys')}
            testID="settings-keys"
          />
          <Row
            icon="log-out-outline"
            label={t('settings.signout')}
            onPress={() => void signOut()}
            testID="settings-signout"
            last
          />
        </View>

        <Text style={styles.section}>{t('settings.storage')}</Text>
        <View style={[styles.card, { backgroundColor: colors.surface }]}>
          {storage ? (
            <View style={[styles.hub, { borderBottomColor: colors.hairline }]} testID="settings-storage-hub">
              <Ionicons name="cloud-outline" size={22} color={colors.textSecondary} />
              <Text style={[styles.hubText, { color: colors.text }]}>{storageLine(storage)}</Text>
            </View>
          ) : null}
          <Row
            icon="images-outline"
            label={cleared ? t('settings.storage.cleared') : t('settings.storage.clear')}
            onPress={clearMedia}
            testID="settings-clear-media"
            last
          />
        </View>
        <Text style={styles.note}>{t('settings.storage.note')}</Text>

        <Text style={styles.section}>{t('settings.about')}</Text>
        <View style={[styles.card, { backgroundColor: colors.surface }]}>
          <Row
            icon="logo-github"
            label={t('settings.about.source')}
            onPress={() => void openLink(SOURCE)}
            testID="settings-source"
            last
          />
        </View>
        <Text style={styles.note}>{t('settings.about.version', { version: VERSION })}</Text>

        <Pressable
          accessibilityRole="button"
          onPress={() => setConfirm('delete')}
          style={styles.danger}
          testID="settings-delete-account"
        >
          <Text style={[styles.dangerText, { color: colors.danger }]}>{t('settings.delete')}</Text>
        </Pressable>
      </ScrollView>
      <ConfirmSheet
        visible={confirm === 'delete'}
        title={t('settings.delete.title')}
        body={t('settings.delete.body')}
        confirmLabel={t('settings.delete')}
        destructive
        busy={busy}
        onConfirm={() => void deleteAccount()}
        onCancel={() => setConfirm(null)}
      />
    </Screen>
  );
}

function Row({
  icon,
  label,
  onPress,
  testID,
  last,
}: {
  icon: keyof typeof Ionicons.glyphMap;
  label: string;
  onPress: () => void;
  testID: string;
  last?: boolean;
}) {
  const { colors } = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      testID={testID}
      style={({ pressed }) => [
        row.base,
        !last && { borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: colors.hairline },
        pressed && { backgroundColor: colors.surfaceStrong },
      ]}
    >
      <Ionicons name={icon} size={22} color={colors.accent} />
      <Text style={[row.label, { color: colors.text }]}>{label}</Text>
      <Ionicons name="chevron-forward" size={18} color={colors.textSecondary} />
    </Pressable>
  );
}

const row = StyleSheet.create({
  base: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
  },
  label: { ...type.body, flex: 1 },
});

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl, gap: spacing.sm },
    section: {
      ...type.label,
      color: colors.textSecondary,
      textTransform: 'uppercase',
      letterSpacing: 0.4,
      marginTop: spacing.md,
    },
    pills: { flexDirection: 'row', gap: spacing.sm },
    pill: { flex: 1, alignItems: 'center', paddingVertical: spacing.sm + 2, borderRadius: radius.pill },
    pillText: { ...type.secondary, fontWeight: '600' },
    card: { borderRadius: radius.lg, overflow: 'hidden' },
    // The hub's line sits in the card like a row, but is not one to tap.
    hub: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md,
      paddingHorizontal: spacing.md,
      paddingVertical: spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
    },
    hubText: { ...type.secondary, color: colors.text, flex: 1, lineHeight: 20 },
    note: { ...type.caption, color: colors.textSecondary, lineHeight: 18 },
    danger: { alignItems: 'center', paddingVertical: spacing.lg, marginTop: spacing.lg },
    dangerText: { ...type.body, fontWeight: '600' },
  });
