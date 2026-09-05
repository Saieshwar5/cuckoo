import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { ApiError } from '@/api/client';
import { Avatar } from '@/components/Avatar';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { sizes, spacing, type, useStyles, type Theme } from '@/theme';

// Profile setup: the one thing a new account is asked for. The avatar
// above the field takes the name's colour and initials as it is typed.
export default function ProfileSetupScreen() {
  const { api, setUser } = useSession();
  const router = useRouter();
  const styles = useStyles(makeStyles);
  // Empty on purpose: the hub's default is made from the email address,
  // and this screen exists to replace it with a name the person chose.
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const save = async () => {
    if (!name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      const updated = await api.updateMe({ display_name: name.trim() });
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
        <View style={styles.avatar}>
          <Avatar name={name.trim() || '?'} size={sizes.avatarLarge} />
        </View>
        <Text style={styles.title}>{t('profile.title')}</Text>
        <Text style={styles.subtitle}>{t('profile.subtitle')}</Text>
        <TextField
          label={t('profile.name.label')}
          icon="person-outline"
          value={name}
          onChangeText={setName}
          autoFocus
          autoCapitalize="words"
          returnKeyType="done"
          onSubmitEditing={() => void save()}
          error={error}
          testID="name"
        />
        <Button title={t('profile.save')} onPress={() => void save()} busy={busy} disabled={!name.trim()} />
      </View>
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
    avatar: { alignItems: 'center', marginBottom: spacing.xl },
    title: { ...type.title, color: colors.text, marginBottom: spacing.xs, textAlign: 'center' },
    subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl, textAlign: 'center' },
  });
