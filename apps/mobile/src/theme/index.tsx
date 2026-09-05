import React, { createContext, useContext, useMemo } from 'react';
import { useColorScheme } from 'react-native';

import { dark, light, type Palette } from './tokens';

export { avatarColors, inputReset, radius, sizes, spacing, tapTarget, type } from './tokens';
export type { Palette } from './tokens';

export type Scheme = 'light' | 'dark';

export interface Theme {
  scheme: Scheme;
  colors: Palette;
}

const themes: Record<Scheme, Theme> = {
  dark: { scheme: 'dark', colors: dark },
  light: { scheme: 'light', colors: light },
};

// Without a provider, as in tests, everything is dark.
const ThemeContext = createContext<Theme>(themes.dark);

// ThemeProvider follows the phone's setting and hands the matching theme to
// everything below it. The scheme is read in one place: no preference reads
// as dark, which is what the phones this is built for are set to.
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const scheme = useColorScheme();
  const theme = themes[scheme === 'light' ? 'light' : 'dark'];
  return <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>;
}

export function useTheme(): Theme {
  return useContext(ThemeContext);
}

// useStyles builds a screen's styles from the theme once per theme. `make`
// lives at module scope and returns StyleSheet.create(...), so a screen
// writes its styles as a function of the palette and nothing else changes.
export function useStyles<T>(make: (theme: Theme) => T): T {
  const theme = useTheme();
  return useMemo(() => make(theme), [make, theme]);
}
