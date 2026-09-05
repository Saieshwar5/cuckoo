import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Platform, StyleSheet, Text, View } from 'react-native';

import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { colors, spacing, type } from '@/theme/tokens';

import { describe } from './sign-in';

// Verify: the code from the email. A right code signs the person in; a new
// account goes to profile setup first.
export default function VerifyScreen() {
  const { email } = useLocalSearchParams<{ email: string }>();
  const { api, signIn } = useSession();
  const router = useRouter();
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [resent, setResent] = useState(false);

  const submit = async () => {
    if (!email) return;
    setBusy(true);
    setError(null);
    try {
      const v = await api.verifySignIn(email, code, deviceName());
      await signIn(v);
      if (v.is_new) router.replace('/profile-setup');
    } catch (err) {
      setError(describe(err));
    } finally {
      setBusy(false);
    }
  };

  const resend = async () => {
    if (!email) return;
    setError(null);
    try {
      await api.startSignIn(email);
      setResent(true);
    } catch (err) {
      setError(describe(err));
    }
  };

  return (
    <Screen>
      <View style={styles.body}>
        <Text style={styles.title}>{t('verify.title')}</Text>
        <Text style={styles.subtitle}>{t('verify.subtitle', { email: email ?? '' })}</Text>
        <TextField
          label={t('verify.code.label')}
          value={code}
          onChangeText={(v) => setCode(v.replace(/\D/g, '').slice(0, 6))}
          keyboardType="number-pad"
          textContentType="oneTimeCode"
          autoComplete="one-time-code"
          maxLength={6}
          returnKeyType="go"
          onSubmitEditing={submit}
          error={error}
          testID="code"
        />
        <Button title={t('common.continue')} onPress={submit} busy={busy} disabled={code.length !== 6} />
        <View style={styles.gap} />
        <Button
          title={resent ? '✓ ' + t('verify.resend') : t('verify.resend')}
          onPress={resend}
          variant="plain"
        />
      </View>
    </Screen>
  );
}

function deviceName(): string {
  return Platform.OS === 'ios' ? 'iPhone' : Platform.OS === 'android' ? 'Android phone' : Platform.OS;
}

const styles = StyleSheet.create({
  body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
  title: { ...type.title, color: colors.text, marginBottom: spacing.sm },
  subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl },
  gap: { height: spacing.sm },
});
