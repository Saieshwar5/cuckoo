import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { ApiError } from '@/api/client';
import type { Cadence, Repeat, Weekday } from '@/api/types';
import { useConversation } from '@/chats/useChats';
import { Button } from '@/components/Button';
import { counterpart } from '@/components/ChatRow';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { clock, dateLabel, deviceZone, WEEKDAYS } from '@/schedules/format';
import { pickDate, pickTime } from '@/schedules/pick';
import { useSchedules } from '@/schedules/useSchedules';
import { radius, spacing, type, useTheme } from '@/theme';

const REPEATS: Repeat[] = ['daily', 'weekdays', 'weekly', 'once'];

function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// Making or changing a schedule: what, in the person's words, and when, as
// a repeat and a time picked on the phone's own clock. Nothing reads "7" and
// decides it meant the evening.
export default function EditScheduleScreen() {
  const params = useLocalSearchParams<{ conversation: string; schedule?: string }>();
  const conversationId = params.conversation;
  const router = useRouter();
  const { colors } = useTheme();
  const conversation = useConversation(conversationId);
  const name = conversation ? (counterpart(conversation)?.display_name ?? '') : '';
  const { schedules, request, update, remove } = useSchedules(conversationId);
  const existing = schedules.find((s) => s.id === params.schedule);

  const [instruction, setInstruction] = useState<string | null>(null);
  const [cadence, setCadence] = useState<Cadence | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  // The form starts from the schedule once it has loaded, and from a
  // morning default for a new one.
  const words = instruction ?? existing?.instruction ?? '';
  const when: Cadence = cadence ??
    existing?.cadence ?? { repeat: 'daily', time: '07:00', timezone: deviceZone() };
  const change = (next: Partial<Cadence>) => setCadence({ ...when, ...next });

  const back = () =>
    router.canGoBack()
      ? router.back()
      : router.replace({ pathname: '/schedules/[id]', params: { id: conversationId } });

  const ready =
    words.trim().length > 0 &&
    (when.repeat !== 'weekly' || (when.days?.length ?? 0) > 0) &&
    (when.repeat !== 'once' || !!when.date);

  const save = async () => {
    setBusy(true);
    setError(null);
    const clean: Cadence = {
      repeat: when.repeat,
      time: when.time,
      timezone: when.timezone,
      ...(when.repeat === 'weekly' ? { days: when.days } : {}),
      ...(when.repeat === 'once' ? { date: when.date } : {}),
    };
    try {
      if (existing) await update(existing.id, { instruction: words.trim(), cadence: clean });
      else await request({ instruction: words.trim(), cadence: clean });
      back();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t('schedules.error'));
      setBusy(false);
    }
  };

  const destroy = async () => {
    if (!existing) return;
    setBusy(true);
    try {
      await remove(existing.id);
      setConfirmDelete(false);
      back();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : t('schedules.error'));
      setBusy(false);
    }
  };

  const chip = (label: string, on: boolean, onPress: () => void, testID: string) => (
    <Pressable
      key={testID}
      accessibilityRole="button"
      accessibilityState={{ selected: on }}
      onPress={onPress}
      style={[styles.chip, { backgroundColor: on ? colors.accent : colors.surface }]}
      testID={testID}
    >
      <Text style={[styles.chipText, { color: on ? colors.onAccent : colors.text }]}>{label}</Text>
    </Pressable>
  );

  const toggleDay = (d: Weekday) => {
    const days = when.days ?? [];
    change({
      days: days.includes(d)
        ? days.filter((x) => x !== d)
        : WEEKDAYS.filter((x) => x === d || days.includes(x)),
    });
  };

  return (
    <Screen padded={false}>
      <TopBar title={existing ? t('schedules.edit') : t('schedules.new')} onBack={back} />
      <ScrollView contentContainerStyle={styles.form} keyboardShouldPersistTaps="handled">
        <TextField
          label={t('schedules.what', { name })}
          value={words}
          onChangeText={setInstruction}
          placeholder={t('schedules.what.placeholder')}
          multiline
          maxLength={500}
          testID="schedule-instruction"
        />

        <Text style={[styles.label, { color: colors.textSecondary }]}>{t('schedules.repeat')}</Text>
        <View style={styles.chips}>
          {REPEATS.map((r) =>
            chip(
              t(`schedules.repeat.${r}`),
              when.repeat === r,
              () => change({ repeat: r, ...(r === 'once' && !when.date ? { date: today() } : {}) }),
              `repeat-${r}`,
            ),
          )}
        </View>

        {when.repeat === 'weekly' ? (
          <View style={styles.chips}>
            {WEEKDAYS.map((d) =>
              chip(t(`schedules.day.${d}`), (when.days ?? []).includes(d), () => toggleDay(d), `day-${d}`),
            )}
          </View>
        ) : null}

        <View style={styles.pickers}>
          {when.repeat === 'once' ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => pickDate(when.date ?? today(), (date) => change({ date }))}
              style={[styles.picker, { backgroundColor: colors.surface }]}
              testID="schedule-date"
            >
              <Text style={[styles.pickerLabel, { color: colors.textSecondary }]}>{t('schedules.date')}</Text>
              <Text style={[styles.pickerValue, { color: colors.text }]}>
                {dateLabel(when.date ?? today())}
              </Text>
            </Pressable>
          ) : null}
          <Pressable
            accessibilityRole="button"
            onPress={() => pickTime(when.time, (time) => change({ time }))}
            style={[styles.picker, { backgroundColor: colors.surface }]}
            testID="schedule-time"
          >
            <Text style={[styles.pickerLabel, { color: colors.textSecondary }]}>{t('schedules.time')}</Text>
            <Text style={[styles.pickerValue, { color: colors.text }]}>{clock(when.time)}</Text>
          </Pressable>
        </View>
        <Text style={[styles.zone, { color: colors.textSecondary }]}>
          {t('schedules.zone', { zone: when.timezone })}
        </Text>

        {error ? <Text style={[styles.error, { color: colors.danger }]}>{error}</Text> : null}
        <Button
          title={t('schedules.save')}
          onPress={() => void save()}
          disabled={!ready}
          busy={busy}
          testID="schedule-save"
        />
        {existing ? (
          <Button
            title={t('schedules.delete')}
            variant="plain"
            onPress={() => setConfirmDelete(true)}
            disabled={busy}
            testID="schedule-delete"
          />
        ) : null}
        <Text style={[styles.note, { color: colors.textSecondary }]}>{t('schedules.note', { name })}</Text>
      </ScrollView>
      <ConfirmSheet
        visible={confirmDelete}
        title={t('schedules.delete.title')}
        body={t('schedules.delete.body', { name })}
        confirmLabel={t('schedules.delete')}
        destructive
        busy={busy}
        onConfirm={() => void destroy()}
        onCancel={() => setConfirmDelete(false)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  form: { padding: spacing.lg, gap: spacing.md },
  label: { ...type.label, marginTop: spacing.sm },
  chips: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm },
  chip: { paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderRadius: radius.pill },
  chipText: { ...type.secondary, fontWeight: '600' },
  pickers: { flexDirection: 'row', gap: spacing.sm, marginTop: spacing.sm },
  picker: { flex: 1, padding: spacing.md, borderRadius: radius.md, gap: 2 },
  pickerLabel: type.caption,
  pickerValue: { ...type.headline },
  zone: { ...type.caption },
  error: { ...type.secondary },
  note: { ...type.caption, textAlign: 'center', lineHeight: 18, marginTop: spacing.sm },
});
