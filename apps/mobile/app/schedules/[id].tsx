import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Switch, Text, View } from 'react-native';

import type { Schedule } from '@/api/types';
import { useConversation } from '@/chats/useChats';
import { Button } from '@/components/Button';
import { counterpart } from '@/components/ChatRow';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { cadenceLabel, statusLine } from '@/schedules/format';
import { useSchedules } from '@/schedules/useSchedules';
import { radius, spacing, type, useTheme } from '@/theme';

// Ten is what the hub holds for one chat.
const MAX = 10;

// A chat's schedules: what the person asked the agent to do, when, and
// whether it is doing it. The agent runs them; this is where they are seen,
// paused, changed and deleted — and where a miss is admitted rather than
// hidden.
export default function SchedulesScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { colors } = useTheme();
  const conversation = useConversation(id);
  const name = conversation ? (counterpart(conversation)?.display_name ?? '') : '';
  const { schedules, loading, update } = useSchedules(id);
  const [busyId, setBusyId] = useState<string | null>(null);
  const now = new Date();

  const back = () =>
    router.canGoBack() ? router.back() : router.replace({ pathname: '/chat/[id]', params: { id } });
  const open = (scheduleId?: string) =>
    router.push({
      pathname: '/schedules/edit',
      params: scheduleId ? { conversation: id, schedule: scheduleId } : { conversation: id },
    });
  const togglePaused = async (s: Schedule) => {
    setBusyId(s.id);
    try {
      await update(s.id, { paused: s.status !== 'paused' });
    } finally {
      setBusyId(null);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('schedules.title')} onBack={back} />
      {loading ? (
        <ActivityIndicator color={colors.accent} style={styles.loading} />
      ) : schedules.length === 0 ? (
        <EmptyState
          icon="alarm-outline"
          title={t('schedules.empty.title')}
          subtitle={t('schedules.empty.subtitle', { name })}
        />
      ) : (
        <ScrollView contentContainerStyle={styles.list}>
          <Text style={[styles.lead, { color: colors.textSecondary }]}>{t('schedules.lead', { name })}</Text>
          {schedules.map((s) => {
            const line = statusLine(s, name, now);
            return (
              <Pressable
                key={s.id}
                accessibilityRole="button"
                onPress={() => open(s.id)}
                style={({ pressed }) => [
                  styles.row,
                  { backgroundColor: pressed ? colors.surfaceStrong : colors.surface },
                ]}
                testID={`schedule-${s.id}`}
              >
                <View style={[styles.icon, { backgroundColor: colors.accentTint }]}>
                  <Ionicons name="alarm-outline" size={20} color={colors.accentStrong} />
                </View>
                <View style={styles.body}>
                  <Text style={[styles.name, { color: colors.text }]} numberOfLines={1}>
                    {s.title}
                  </Text>
                  <Text style={[styles.when, { color: colors.textSecondary }]} numberOfLines={1}>
                    {cadenceLabel(s.cadence)}
                  </Text>
                  <Text
                    style={[
                      styles.status,
                      {
                        color:
                          line.tone === 'warn'
                            ? colors.danger
                            : line.tone === 'quiet'
                              ? colors.textSecondary
                              : colors.accentStrong,
                      },
                    ]}
                    numberOfLines={1}
                    testID={`schedule-status-${s.id}`}
                  >
                    {line.text}
                  </Text>
                </View>
                <Switch
                  value={s.status !== 'paused'}
                  onValueChange={() => void togglePaused(s)}
                  disabled={busyId === s.id}
                  trackColor={{ true: colors.accent, false: colors.surfaceStrong }}
                  accessibilityLabel={s.status === 'paused' ? t('schedules.resume') : t('schedules.pause')}
                  testID={`schedule-toggle-${s.id}`}
                />
              </Pressable>
            );
          })}
        </ScrollView>
      )}
      <View style={[styles.footer, { borderTopColor: colors.hairline }]}>
        {schedules.length >= MAX ? (
          <Text style={[styles.limit, { color: colors.textSecondary }]}>
            {t('schedules.limit', { max: MAX })}
          </Text>
        ) : (
          <Button title={t('schedules.new')} onPress={() => open()} testID="schedule-new" />
        )}
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  loading: { marginTop: spacing.xl },
  list: { padding: spacing.lg, gap: spacing.sm },
  lead: { ...type.secondary, marginBottom: spacing.sm, lineHeight: 20 },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    padding: spacing.md,
    borderRadius: radius.md,
  },
  icon: { width: 40, height: 40, borderRadius: 20, alignItems: 'center', justifyContent: 'center' },
  body: { flex: 1, gap: 2 },
  name: type.headline,
  when: type.secondary,
  status: { ...type.caption, fontWeight: '600' },
  footer: { padding: spacing.lg, borderTopWidth: StyleSheet.hairlineWidth },
  limit: { ...type.secondary, textAlign: 'center' },
});
