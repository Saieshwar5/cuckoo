import React, { useEffect, useState } from 'react';
import { Animated, StyleSheet, View } from 'react-native';

// WorkingDots is three small dots taking turns: an agent busy, said in a
// space the size of a word. The chat list's rows and the chat's header use
// it beside the words that say what the work is.
export function WorkingDots({ color, size = 5 }: { color: string; size?: number }) {
  const [dots] = useState(() => [
    new Animated.Value(0.25),
    new Animated.Value(0.25),
    new Animated.Value(0.25),
  ]);

  useEffect(() => {
    const pulse = (v: Animated.Value, delay: number) =>
      Animated.sequence([
        Animated.delay(delay),
        Animated.timing(v, { toValue: 1, duration: 280, useNativeDriver: false }),
        Animated.timing(v, { toValue: 0.25, duration: 280, useNativeDriver: false }),
      ]);
    const loop = Animated.loop(
      Animated.parallel(dots.map((v, i) => pulse(v, i * 150)).concat(Animated.delay(180))),
    );
    loop.start();
    return () => loop.stop();
  }, [dots]);

  return (
    <View style={[styles.row, { gap: size * 0.6 }]} testID="working-dots">
      {dots.map((opacity, i) => (
        <Animated.View
          key={i}
          style={{ width: size, height: size, borderRadius: size / 2, backgroundColor: color, opacity }}
        />
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center' },
});
