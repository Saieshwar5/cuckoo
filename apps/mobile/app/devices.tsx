import { useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { describeAgentError } from '@/agents/form';
import { ApiError } from '@/api/client';
import type { Device } from '@/api/types';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { deviceLine, deviceName, orderDevices } from '@/devices/format';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useStyles, useTheme, type Theme } from '@/theme';

// What the person is asked to confirm: one device, or all the others.
type Pending = { kind: 'one'; device: Device } | { kind: 'others' };

// Devices: where this person is signed in. Several devices at once are
// allowed, so the way out of any of them has to be here rather than only
// in signing in again somewhere else.
export default function DevicesScreen() {
  const router = useRouter();
  const { api } = useSession();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [devices, setDevices] = useState<Device[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<Pending | null>(null);

  const load = useCallback(
    () =>
      api.listDevices().then(
        (list) => {
          setDevices(orderDevices(list));
          setLoading(false);
        },
        () => setLoading(false),
      ),
    [api],
  );

  useEffect(() => {
    load();
  }, [load]);

  const signOut = async () => {
    if (!confirm) return;
    setBusy(true);
    setError(null);
    try {
      if (confirm.kind === 'one') await api.signOutDevice(confirm.device.id);
      else await api.signOutOtherDevices();
      setConfirm(null);
    } catch (err) {
      // A device that is already signed out is not a failure worth a
      // sentence: the list about to be reloaded is the answer.
      if (err instanceof ApiError && err.code === 'device_not_found') setConfirm(null);
      else setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
      await load();
    }
  };

  const others = devices.filter((d) => !d.current).length;

  return (
    <Screen padded={false}>
      <TopBar title={t('devices.title')} onBack={() => router.back()} />
      <FlatList
        data={devices}
        keyExtractor={(d) => d.id}
        ListHeaderComponent={
          <View style={styles.head}>
            <Text style={styles.subtitle}>{t('devices.subtitle')}</Text>
            {error ? <Text style={styles.error}>{error}</Text> : null}
          </View>
        }
        renderItem={({ item }) => (
          <View style={styles.row}>
            <View style={styles.body}>
              <View style={styles.nameRow}>
                <Text style={styles.name} numberOfLines={1}>
                  {deviceName(item)}
                </Text>
                {item.current ? (
                  <View style={[styles.badge, { backgroundColor: colors.surface }]} testID="badge-current">
                    <Text style={styles.badgeText}>{t('devices.current')}</Text>
                  </View>
                ) : null}
              </View>
              <Text style={styles.meta} numberOfLines={1}>
                {deviceLine(item)}
              </Text>
            </View>
            {item.current ? null : (
              <Pressable
                accessibilityRole="button"
                onPress={() => setConfirm({ kind: 'one', device: item })}
                hitSlop={8}
                testID={`signout-${item.id}`}
              >
                <Text style={styles.signout}>{t('devices.signout')}</Text>
              </Pressable>
            )}
          </View>
        )}
        ListFooterComponent={
          others > 0 ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => setConfirm({ kind: 'others' })}
              style={styles.all}
              testID="signout-others"
            >
              <Text style={styles.allText}>{t('devices.others')}</Text>
            </Pressable>
          ) : null
        }
        refreshControl={
          <RefreshControl refreshing={loading} onRefresh={() => void load()} tintColor={colors.accent} />
        }
        ListEmptyComponent={
          loading ? null : (
            <EmptyState
              icon="phone-portrait-outline"
              title={t('devices.empty.title')}
              subtitle={t('devices.empty.subtitle')}
            />
          )
        }
        contentContainerStyle={styles.list}
      />
      <ConfirmSheet
        visible={confirm !== null}
        title={
          confirm?.kind === 'one'
            ? t('devices.signout.title', { name: deviceName(confirm.device) })
            : t('devices.others.title')
        }
        body={confirm?.kind === 'one' ? t('devices.signout.body') : t('devices.others.body')}
        confirmLabel={confirm?.kind === 'one' ? t('devices.signout') : t('devices.others')}
        destructive
        busy={busy}
        onConfirm={() => void signOut()}
        onCancel={() => setConfirm(null)}
      />
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    list: { paddingBottom: spacing.xxl },
    head: { padding: spacing.lg, gap: spacing.sm },
    subtitle: { ...type.body, color: colors.textSecondary, lineHeight: 22 },
    error: { ...type.secondary, color: colors.danger },
    row: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md,
      paddingHorizontal: spacing.lg,
      paddingVertical: spacing.md,
    },
    body: { flex: 1, gap: 3 },
    nameRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    name: { ...type.headline, color: colors.text, flexShrink: 1 },
    badge: { borderRadius: radius.pill, paddingHorizontal: spacing.sm, paddingVertical: 2 },
    badgeText: { ...type.caption, color: colors.textSecondary },
    meta: { ...type.secondary, color: colors.textSecondary },
    signout: {
      ...type.secondary,
      color: colors.danger,
      fontWeight: '600',
      paddingHorizontal: spacing.sm,
    },
    all: { alignItems: 'center', paddingVertical: spacing.lg, marginTop: spacing.md },
    allText: { ...type.body, color: colors.danger, fontWeight: '600' },
  });
