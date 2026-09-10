import { KeyboardAvoidingView } from 'react-native-keyboard-controller';
import React from 'react';
import { StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { spacing, useTheme } from '@/theme';

// Screen is the ground every screen stands on: safe area, the theme's ground
// colour, and a keyboard that pushes content up instead of covering it.
//
// The keyboard part was a lie until somebody held a phone. React Native's own
// KeyboardAvoidingView was given `behavior` on iOS and `undefined` on Android,
// and with no behavior it does nothing at all — so on Android the keyboard
// slid over the composer and you typed into a box you could not see. It was
// never noticed because the app is developed in a browser, and browsers have
// no keyboard.
//
// This one is from react-native-keyboard-controller, which drives the same
// animation the keyboard itself is using on both platforms. `padding` on both,
// because there is no longer a platform that needs to be treated as the
// exception.
export function Screen({ children, padded = true }: { children: React.ReactNode; padded?: boolean }) {
  const { colors } = useTheme();
  return (
    <SafeAreaView style={[styles.flex, { backgroundColor: colors.ground }]} edges={['top', 'left', 'right']}>
      <KeyboardAvoidingView style={styles.flex} behavior="padding">
        <View style={[styles.flex, padded && styles.padded]}>{children}</View>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  padded: { paddingHorizontal: spacing.lg },
});
