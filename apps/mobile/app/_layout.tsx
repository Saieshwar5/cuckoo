import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import React from 'react';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { ChatsProvider } from '@/chats/ChatsProvider';
import { RealtimeProvider } from '@/realtime/RealtimeProvider';
import { SessionProvider, useSession } from '@/session/SessionProvider';
import { ThemeProvider, useTheme } from '@/theme';

// The root: a session, and a stack whose halves are guarded by it. A signed
// out person can only reach sign-in; a signed-in one can never see it.
export default function RootLayout() {
  return (
    <ThemeProvider>
      <SessionProvider>
        <RealtimeProvider>
          <ChatsProvider>
            <Routes />
          </ChatsProvider>
        </RealtimeProvider>
      </SessionProvider>
    </ThemeProvider>
  );
}

function Routes() {
  const { status } = useSession();
  const { scheme, colors } = useTheme();
  const bar = <StatusBar style={scheme === 'dark' ? 'light' : 'dark'} />;
  if (status === 'loading') {
    return (
      <View style={[styles.loading, { backgroundColor: colors.ground }]}>
        {bar}
        <ActivityIndicator color={colors.accent} />
      </View>
    );
  }
  return (
    <>
      {bar}
      <Stack screenOptions={{ headerShown: false, contentStyle: { backgroundColor: colors.ground } }}>
        <Stack.Protected guard={status === 'signedOut'}>
          <Stack.Screen name="(auth)" />
        </Stack.Protected>
        <Stack.Protected guard={status === 'signedIn'}>
          <Stack.Screen name="(tabs)" />
          <Stack.Screen name="profile-setup" />
          <Stack.Screen name="chat/[id]" />
        </Stack.Protected>
      </Stack>
    </>
  );
}

const styles = StyleSheet.create({
  loading: { flex: 1, alignItems: 'center', justifyContent: 'center' },
});
