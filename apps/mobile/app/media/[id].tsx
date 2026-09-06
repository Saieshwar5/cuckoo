import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React from 'react';
import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { t } from '@/i18n';
import { openAttachment } from '@/media/save';
import { useMediaSource } from '@/media/source';
import { useSession } from '@/session/SessionProvider';
import { spacing, type } from '@/theme';
import { dark } from '@/theme/tokens';

// One picture, as large as the screen allows.
//
// Always on black, in either theme: a photograph is looked at against
// nothing, and the chrome steps back to the corners so it is not part of
// what you are looking at.
export default function MediaScreen() {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { api, token } = useSession();
  const { id, name } = useLocalSearchParams<{ id: string; name?: string }>();
  const source = useMediaSource(id);
  const fileName = name ?? '';

  return (
    <View style={styles.screen}>
      {source ? (
        <Image
          source={source}
          style={styles.image}
          resizeMode="contain"
          accessibilityLabel={fileName}
          accessibilityIgnoresInvertColors
          testID="media-image"
        />
      ) : (
        <ActivityIndicator color={dark.text} />
      )}

      <View style={[styles.bar, { paddingTop: insets.top + spacing.sm }]}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('chat.back')}
          onPress={() => router.back()}
          hitSlop={12}
          testID="media-close"
        >
          <Ionicons name="close" size={26} color={dark.text} />
        </Pressable>
        <Text style={styles.name} numberOfLines={1}>
          {fileName}
        </Text>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('chat.file.save')}
          onPress={() => void openAttachment(api, id, fileName || 'photo.jpg', token)}
          hitSlop={12}
          testID="media-save"
        >
          <Ionicons name="download-outline" size={24} color={dark.text} />
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#000000', alignItems: 'center', justifyContent: 'center' },
  image: { width: '100%', height: '100%' },
  bar: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
  },
  name: { ...type.secondary, color: dark.text, flex: 1 },
});
