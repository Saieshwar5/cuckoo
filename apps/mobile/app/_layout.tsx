import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import React from 'react';
import { ActivityIndicator, StyleSheet, View } from 'react-native';

import { SessionProvider, useSession } from '@/session/SessionProvider';
import { ThemeProvider, type, useTheme } from '@/theme';

// The root: a session, and a stack whose halves are guarded by it. A signed
// out person can only reach sign-in; a signed-in one can never see it.
export default function RootLayout() {
  return (
    <ThemeProvider>
      <SessionProvider>
        <Routes />
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
          <Stack.Screen
            name="chat/[id]"
            options={{
              headerShown: true,
              headerBackTitle: '',
              headerStyle: { backgroundColor: colors.surface },
              headerTintColor: colors.text,
              headerTitleStyle: { ...type.headline, color: colors.text },
              headerShadowVisible: false,
            }}
          />
        </Stack.Protected>
      </Stack>
    </>
  );
}

const styles = StyleSheet.create({
  loading: { flex: 1, alignItems: 'center', justifyContent: 'center' },
});
