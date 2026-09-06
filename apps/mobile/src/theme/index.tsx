import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { useColorScheme } from 'react-native';

import { loadPreference, savePreference, type Preference } from './preference';
import { dark, light, type Palette } from './tokens';

export { inputReset, radius, sizes, spacing, tapTarget, type } from './tokens';
export type { Palette } from './tokens';
export type { Preference } from './preference';

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

// What the person chose in Settings, and how to change it.
interface PreferenceState {
  preference: Preference;
  setPreference: (p: Preference) => void;
}

const PreferenceContext = createContext<PreferenceState>({ preference: 'system', setPreference: () => {} });

// ThemeProvider follows the phone's setting unless the person chose
// otherwise in Settings, and hands the matching theme to everything below
// it. The scheme is read in one place: no preference reads as dark, which is
// what the phones this is built for are set to.
export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const device = useColorScheme();
  const [preference, setPreferenceState] = useState<Preference>('system');
  useEffect(() => {
    let cancelled = false;
    void loadPreference().then((p) => {
      if (!cancelled) setPreferenceState(p);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  const setPreference = useCallback((p: Preference) => {
    setPreferenceState(p);
    void savePreference(p);
  }, []);
  const scheme: Scheme = preference === 'system' ? (device === 'light' ? 'light' : 'dark') : preference;
  const theme = themes[scheme];
  const state = useMemo(() => ({ preference, setPreference }), [preference, setPreference]);
  return (
    <PreferenceContext.Provider value={state}>
      <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>
    </PreferenceContext.Provider>
  );
}

export function usePreference(): PreferenceState {
  return useContext(PreferenceContext);
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
