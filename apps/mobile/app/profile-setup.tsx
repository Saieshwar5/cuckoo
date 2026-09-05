import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { ApiError } from '@/api/client';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { colors, spacing, type } from '@/theme/tokens';

// Profile setup: the one thing a new account is asked for.
export default function ProfileSetupScreen() {
  const { api, user, setUser } = useSession();
  const router = useRouter();
  const [name, setName] = useState(user?.display_name ?? '');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      const updated = await api.updateMe({ display_name: name });
      await setUser({ id: updated.id, display_name: updated.display_name });
      router.replace('/(tabs)/chats');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t('common.error.unknown'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Screen>
      <View style={styles.body}>
        <Text style={styles.title}>{t('profile.title')}</Text>
        <Text style={styles.subtitle}>{t('profile.subtitle')}</Text>
        <TextField
          label={t('profile.name.label')}
          value={name}
          onChangeText={setName}
          autoFocus
          returnKeyType="done"
          onSubmitEditing={save}
          error={error}
          testID="name"
        />
        <Button title={t('profile.save')} onPress={save} busy={busy} disabled={!name.trim()} />
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
  title: { ...type.title, color: colors.text, marginBottom: spacing.sm },
  subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl },
});
