import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { Button } from '@/components/Button';
import { CodeInput } from '@/components/CodeInput';
import { Screen } from '@/components/Screen';
import { useCountdown } from '@/hooks/useCountdown';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { spacing, type, useStyles, type Theme } from '@/theme';
import { formatCountdown } from '@/util/time';

import { describe } from './sign-in';

// Verify: the code from the email. The sixth digit submits on its own; a
// right code signs the person in, and a new account goes to profile setup.
export default function VerifyScreen() {
  const { email } = useLocalSearchParams<{ email: string }>();
  const { api, signIn } = useSession();
  const router = useRouter();
  const styles = useStyles(makeStyles);
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [resent, setResent] = useState(false);
  const { remaining, restart } = useCountdown(30);
  // The sentence around the address, so the address alone can be bold.
  const [before, after] = t('verify.subtitle', { email: '\u0000' }).split('\u0000');

  const submit = async (value: string) => {
    if (!email || busy) return;
    setBusy(true);
    setError(null);
    try {
      const v = await api.verifySignIn(email, value, deviceName());
      await signIn(v);
      if (v.is_new) router.replace('/profile-setup');
    } catch (err) {
      setError(describe(err));
      setCode('');
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
      restart();
    } catch (err) {
      setError(describe(err));
    }
  };

  return (
    <Screen>
      <View style={styles.body}>
        <Text style={styles.title}>{t('verify.title')}</Text>
        <Text style={styles.subtitle}>
          {before}
          <Text style={styles.email}>{email ?? ''}</Text>
          {after}
        </Text>
        <CodeInput
          label={t('verify.code.label')}
          value={code}
          onChange={(v) => {
            setCode(v);
            setError(null);
          }}
          onComplete={(v) => void submit(v)}
          error={error}
          autoFocus
          testID="code"
        />
        <Button
          title={t('common.continue')}
          onPress={() => void submit(code)}
          busy={busy}
          disabled={code.length !== 6}
        />
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ disabled: remaining > 0 }}
          onPress={() => void resend()}
          disabled={remaining > 0}
          style={styles.resend}
        >
          <Text style={remaining > 0 ? styles.resendWait : styles.resendReady}>
            {remaining > 0
              ? t('verify.resend.wait', { time: formatCountdown(remaining) })
              : t('verify.resend')}
          </Text>
        </Pressable>
        {resent ? <Text style={styles.note}>{t('verify.resent')}</Text> : null}
        <Pressable accessibilityRole="button" onPress={() => router.back()} style={styles.back}>
          <Text style={styles.backText}>{t('verify.back')}</Text>
        </Pressable>
      </View>
    </Screen>
  );
}

function deviceName(): string {
  return Platform.OS === 'ios' ? 'iPhone' : Platform.OS === 'android' ? 'Android phone' : Platform.OS;
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
    title: { ...type.title, color: colors.text, marginBottom: spacing.xs },
    subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl, lineHeight: 22 },
    email: { color: colors.text, fontWeight: '600' },
    resend: { alignSelf: 'center', paddingVertical: spacing.md, minHeight: 44, justifyContent: 'center' },
    resendWait: { ...type.secondary, color: colors.textSecondary },
    resendReady: { ...type.secondary, color: colors.accent, fontWeight: '600' },
    note: { ...type.secondary, color: colors.accent, textAlign: 'center' },
    back: { alignSelf: 'center', paddingVertical: spacing.sm, marginTop: spacing.lg },
    backText: { ...type.secondary, color: colors.textSecondary },
  });
