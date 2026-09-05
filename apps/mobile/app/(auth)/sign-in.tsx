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

// Sign in: an email, and a code on the next screen. Nothing else.
export default function SignInScreen() {
  const { api } = useSession();
  const router = useRouter();
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.startSignIn(email);
      router.push({ pathname: '/verify', params: { email: email.trim().toLowerCase() } });
    } catch (err) {
      setError(describe(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Screen>
      <View style={styles.body}>
        <Text style={styles.title}>{t('signin.title')}</Text>
        <Text style={styles.subtitle}>{t('signin.subtitle')}</Text>
        <TextField
          label={t('signin.email.label')}
          placeholder={t('signin.email.placeholder')}
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          textContentType="emailAddress"
          autoComplete="email"
          returnKeyType="go"
          onSubmitEditing={submit}
          error={error}
          testID="email"
        />
        <Button title={t('common.continue')} onPress={submit} busy={busy} disabled={!email.trim()} />
      </View>
    </Screen>
  );
}

// describe turns a failure into the sentence the person reads.
export function describe(err: unknown): string {
  if (err instanceof ApiError) {
    const known = t(`signin.error.${err.code}`);
    if (known !== `signin.error.${err.code}`) return known;
    const verify = t(`verify.error.${err.code}`);
    if (verify !== `verify.error.${err.code}`) return verify;
    return err.message;
  }
  if (err instanceof Error && err.name === 'NetworkError') return t('common.error.network');
  return t('common.error.unknown');
}

const styles = StyleSheet.create({
  body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
  title: { ...type.title, color: colors.text, marginBottom: spacing.sm },
  subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl },
});
