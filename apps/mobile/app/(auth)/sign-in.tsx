import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { ApiError } from '@/api/client';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { t } from '@/i18n';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useStyles, useTheme, type Theme } from '@/theme';

// Sign in: an email, and a code on the next screen. Nothing else.
export default function SignInScreen() {
  const { api } = useSession();
  const router = useRouter();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!email.trim()) return;
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
        <View style={styles.brand}>
          <View style={styles.mark}>
            <Ionicons name="chatbubble-ellipses" size={40} color={colors.onAccent} />
          </View>
          <Text style={styles.name}>{t('app.name')}</Text>
          <Text style={styles.tagline}>{t('signin.tagline')}</Text>
        </View>
        <Text style={styles.title}>{t('signin.title')}</Text>
        <Text style={styles.subtitle}>{t('signin.subtitle')}</Text>
        <TextField
          label={t('signin.email.label')}
          placeholder={t('signin.email.placeholder')}
          icon="mail-outline"
          value={email}
          onChangeText={(v) => {
            setEmail(v);
            setError(null);
          }}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          textContentType="emailAddress"
          autoComplete="email"
          returnKeyType="go"
          onSubmitEditing={() => void submit()}
          error={error}
          testID="email"
        />
        <Button
          title={t('common.continue')}
          onPress={() => void submit()}
          busy={busy}
          disabled={!email.trim()}
        />
        <Text style={styles.footnote}>{t('signin.footnote')}</Text>
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

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { flex: 1, justifyContent: 'center', paddingBottom: spacing.xxl },
    brand: { alignItems: 'center', marginBottom: spacing.xxl + spacing.md },
    mark: {
      width: 76,
      height: 76,
      borderRadius: radius.xl,
      backgroundColor: colors.accent,
      alignItems: 'center',
      justifyContent: 'center',
      marginBottom: spacing.lg,
    },
    name: { ...type.display, fontSize: 32, color: colors.text },
    tagline: {
      ...type.body,
      color: colors.textSecondary,
      textAlign: 'center',
      marginTop: spacing.sm,
      maxWidth: 280,
    },
    title: { ...type.title, color: colors.text, marginBottom: spacing.xs },
    subtitle: { ...type.body, color: colors.textSecondary, marginBottom: spacing.xl },
    footnote: { ...type.secondary, color: colors.textSecondary, textAlign: 'center', marginTop: spacing.xl },
  });
