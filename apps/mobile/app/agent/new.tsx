import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ScrollView, StyleSheet, Text, View } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import { describeAgentError } from '@/agents/form';
import { Avatar } from '@/components/Avatar';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { sizes, spacing, type, useStyles, type Theme } from '@/theme';
import { isValidHandle, suggestHandle } from '@/util/handle';

// New agent: a name, a handle suggested from it, a line about it. It exists
// the moment it is created; connecting a backend is the next screen.
export default function NewAgentScreen() {
  const router = useRouter();
  const { controller } = useAgents();
  const styles = useStyles(makeStyles);
  const [name, setName] = useState('');
  const [handle, setHandle] = useState('');
  const [handleEdited, setHandleEdited] = useState(false);
  const [description, setDescription] = useState('');
  const [error, setError] = useState<{ message: string; field: string | null } | null>(null);
  const [busy, setBusy] = useState(false);

  const changeName = (v: string) => {
    setName(v);
    if (!handleEdited) setHandle(suggestHandle(v));
    setError(null);
  };
  const changeHandle = (v: string) => {
    setHandle(v.toLowerCase().replace(/\s+/g, '-'));
    setHandleEdited(v.length > 0);
    setError(null);
  };

  const ready = name.trim().length > 0 && isValidHandle(handle);

  const create = async () => {
    if (!ready || !controller) return;
    setBusy(true);
    setError(null);
    try {
      const agent = await controller.create({
        handle,
        display_name: name.trim(),
        description: description.trim(),
      });
      router.replace({ pathname: '/agent/[id]', params: { id: agent.id } });
    } catch (err) {
      setError(describeAgentError(err));
      setBusy(false);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('agent.new.title')} onBack={() => router.back()} />
      <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
        <View style={styles.avatar}>
          <Avatar name={name.trim() || '?'} size={sizes.avatarLarge} />
        </View>
        <Text style={styles.subtitle}>{t('agent.new.subtitle')}</Text>
        <TextField
          label={t('agent.name.label')}
          placeholder={t('agent.name.placeholder')}
          value={name}
          onChangeText={changeName}
          autoFocus
          autoCapitalize="words"
          error={error?.field === 'name' ? error.message : null}
          testID="agent-name"
        />
        <TextField
          label={t('agent.handle.label')}
          value={handle}
          onChangeText={changeHandle}
          autoCapitalize="none"
          autoCorrect={false}
          icon="at-outline"
          error={
            error?.field === 'handle'
              ? error.message
              : handle && !isValidHandle(handle)
                ? t('agent.error.invalid_handle')
                : null
          }
          testID="agent-handle"
        />
        <Text style={styles.hint}>{t('agent.handle.hint')}</Text>
        <TextField
          label={t('agent.description.label')}
          placeholder={t('agent.description.placeholder')}
          value={description}
          onChangeText={setDescription}
          multiline
          error={error?.field === 'description' ? error.message : null}
          testID="agent-description"
        />
        {error && !error.field ? <Text style={styles.error}>{error.message}</Text> : null}
        <Button
          title={t('agent.create')}
          onPress={() => void create()}
          busy={busy}
          disabled={!ready}
          testID="agent-create"
        />
      </ScrollView>
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl },
    avatar: { alignItems: 'center', marginBottom: spacing.lg },
    subtitle: { ...type.body, color: colors.textSecondary, textAlign: 'center', marginBottom: spacing.xl },
    hint: {
      ...type.caption,
      color: colors.textSecondary,
      marginTop: -spacing.sm,
      marginBottom: spacing.lg,
      lineHeight: 16,
    },
    error: { ...type.secondary, color: colors.danger, marginBottom: spacing.md },
  });
