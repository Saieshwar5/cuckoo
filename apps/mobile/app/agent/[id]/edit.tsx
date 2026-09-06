import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ScrollView, StyleSheet, Text } from 'react-native';

import { useAgent, useAgents } from '@/agents/AgentsProvider';
import { describeAgentError } from '@/agents/form';
import { AvatarPicker } from '@/components/AvatarPicker';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import { agentAvatar } from '@/media/avatar';
import type { PickedFile } from '@/media/pick';
import { spacing, type, useStyles, type Theme } from '@/theme';

// Edit agent: the name and the line about it. The handle is shown and does
// not change; it is the agent's address.
export default function EditAgentScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const agent = useAgent(id);
  const { controller } = useAgents();
  const styles = useStyles(makeStyles);
  const [name, setName] = useState(agent?.display_name ?? '');
  const [description, setDescription] = useState(agent?.description ?? '');
  const [picture, setPicture] = useState<PickedFile | null>(null);
  const [error, setError] = useState<{ message: string; field: string | null } | null>(null);
  const [busy, setBusy] = useState(false);

  const changed = agent
    ? name.trim() !== agent.display_name || description.trim() !== agent.description || !!picture
    : false;

  const save = async () => {
    if (!agent || !controller || !changed || !name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await controller.update(
        agent.id,
        { display_name: name.trim(), description: description.trim() },
        picture,
      );
      router.back();
    } catch (err) {
      setError(describeAgentError(err));
      setBusy(false);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('agent.edit.title')} onBack={() => router.back()} />
      <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
        <AvatarPicker
          name={name.trim() || '?'}
          source={agent ? agentAvatar(agent.id, agent.has_avatar) : null}
          onPicked={setPicture}
        />
        <TextField
          label={t('agent.name.label')}
          value={name}
          onChangeText={(v) => {
            setName(v);
            setError(null);
          }}
          autoCapitalize="words"
          error={error?.field === 'name' ? error.message : null}
          testID="agent-name"
        />
        <TextField
          label={t('agent.handle.label')}
          value={agent?.handle ?? ''}
          editable={false}
          icon="at-outline"
        />
        <Text style={styles.hint}>{t('agent.handle.fixed')}</Text>
        <TextField
          label={t('agent.description.label')}
          value={description}
          onChangeText={setDescription}
          multiline
          error={error?.field === 'description' ? error.message : null}
          testID="agent-description"
        />
        {error && !error.field ? <Text style={styles.error}>{error.message}</Text> : null}
        <Button
          title={t('agent.save')}
          onPress={() => void save()}
          busy={busy}
          disabled={!changed || !name.trim()}
          testID="agent-save"
        />
      </ScrollView>
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl },
    hint: { ...type.caption, color: colors.textSecondary, marginTop: -spacing.sm, marginBottom: spacing.lg },
    error: { ...type.secondary, color: colors.danger, marginBottom: spacing.md },
  });
