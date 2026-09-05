import { Stack, useLocalSearchParams } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Screen } from '@/components/Screen';
import { t } from '@/i18n';
import { colors, spacing, type } from '@/theme/tokens';

// The conversation. This step opens the door; the next one furnishes the
// room: bubbles, streaming, buttons and the composer.
export default function ChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return (
    <>
      <Stack.Screen options={{ title: t('app.name') }} />
      <Screen>
        <View style={styles.body}>
          <Text style={styles.text}>{t('chat.next')}</Text>
          <Text style={styles.id}>{id}</Text>
        </View>
      </Screen>
    </>
  );
}

const styles = StyleSheet.create({
  body: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: spacing.sm },
  text: { ...type.body, color: colors.textSecondary, textAlign: 'center' },
  id: { ...type.caption, color: colors.textSecondary },
});
