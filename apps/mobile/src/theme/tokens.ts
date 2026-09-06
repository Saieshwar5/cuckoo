import { Platform, type TextStyle } from 'react-native';

// The visual language, as tokens. Two palettes with the same shape, dark
// first because that is how the phones this is built for are set. Only
// white, greys and black: our messages are ink on paper, the agent's are
// paper on ink, and the one thing that gets a colour is an error.

export interface Palette {
  // Backgrounds, from the page up.
  ground: string;
  surface: string;
  surfaceStrong: string;
  hairline: string;
  // Text.
  text: string;
  textSecondary: string;
  // The accent is the opposite of the ground: black on white, white on black.
  accent: string;
  accentStrong: string;
  accentTint: string;
  onAccent: string;
  danger: string;
  // The conversation: its wallpaper and the two kinds of bubble.
  wallpaper: string;
  bubbleMine: string;
  bubbleTheirs: string;
  bubbleBorder: string;
  bubbleTextMine: string;
  bubbleTextTheirs: string;
  bubbleMetaMine: string;
  bubbleMetaTheirs: string;
  bubbleQuoteMine: string;
  bubbleQuoteTheirs: string;
  quoteBarMine: string;
  quoteBarTheirs: string;
  tickRead: string;
  // The dot on every agent avatar.
  statusConnected: string;
  statusIdle: string;
  statusUnreachable: string;
  // Avatar discs: shades of grey a name is mapped onto, and the initials on them.
  avatars: readonly string[];
  onAvatar: string;
  // What is laid over a picture: while it uploads, and behind one opened
  // full-screen. Dark in both themes, because a photograph is looked at
  // against black whatever the rest of the app is doing.
  scrim: string;
}

export const dark: Palette = {
  ground: '#000000',
  surface: '#1A1A1A',
  surfaceStrong: '#2A2A2A',
  hairline: '#262626',
  text: '#F2F2F2',
  textSecondary: '#8E8E8E',
  accent: '#F2F2F2',
  accentStrong: '#F2F2F2',
  accentTint: '#2A2A2A',
  onAccent: '#000000',
  danger: '#FF5A50',
  wallpaper: '#000000',
  bubbleMine: '#E8E8E8',
  bubbleTheirs: '#1A1A1A',
  bubbleBorder: '#262626',
  bubbleTextMine: '#111111',
  bubbleTextTheirs: '#F2F2F2',
  bubbleMetaMine: '#5E5E5E',
  bubbleMetaTheirs: '#8E8E8E',
  bubbleQuoteMine: 'rgba(0, 0, 0, 0.08)',
  bubbleQuoteTheirs: 'rgba(255, 255, 255, 0.08)',
  quoteBarMine: '#111111',
  quoteBarTheirs: '#F2F2F2',
  tickRead: '#F2F2F2',
  statusConnected: '#F2F2F2',
  statusIdle: '#5E5E5E',
  statusUnreachable: '#FF5A50',
  avatars: ['#E8E8E8', '#C4C4C4', '#A0A0A0', '#7C7C7C', '#5A5A5A'],
  onAvatar: '#000000',
  scrim: 'rgba(0, 0, 0, 0.55)',
};

export const light: Palette = {
  ground: '#FFFFFF',
  surface: '#F2F2F2',
  surfaceStrong: '#E4E4E4',
  hairline: '#E6E6E6',
  text: '#111111',
  textSecondary: '#6E6E6E',
  accent: '#111111',
  accentStrong: '#111111',
  accentTint: '#EAEAEA',
  onAccent: '#FFFFFF',
  danger: '#D0342C',
  wallpaper: '#F6F6F6',
  bubbleMine: '#111111',
  bubbleTheirs: '#FFFFFF',
  bubbleBorder: '#E6E6E6',
  bubbleTextMine: '#FFFFFF',
  bubbleTextTheirs: '#111111',
  bubbleMetaMine: 'rgba(255, 255, 255, 0.6)',
  bubbleMetaTheirs: '#8A8A8A',
  bubbleQuoteMine: 'rgba(255, 255, 255, 0.14)',
  bubbleQuoteTheirs: 'rgba(0, 0, 0, 0.05)',
  quoteBarMine: '#FFFFFF',
  quoteBarTheirs: '#111111',
  tickRead: '#111111',
  statusConnected: '#111111',
  statusIdle: '#B0B0B0',
  statusUnreachable: '#D0342C',
  avatars: ['#111111', '#3A3A3A', '#5C5C5C', '#7A7A7A', '#9A9A9A'],
  onAvatar: '#FFFFFF',
  scrim: 'rgba(0, 0, 0, 0.55)',
};

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;

export const radius = { sm: 8, md: 12, lg: 18, xl: 24, pill: 999 } as const;

// The platform's own face, at these sizes. Roboto on Android, San Francisco
// on iOS, the system font on the web: what every other chat app on the
// phone uses, so this one feels native from the first screen.
export const type = {
  display: { fontSize: 28, fontWeight: '700' as const, letterSpacing: -0.3 },
  title: { fontSize: 22, fontWeight: '600' as const },
  headline: { fontSize: 17, fontWeight: '600' as const },
  body: { fontSize: 16, fontWeight: '400' as const },
  secondary: { fontSize: 14, fontWeight: '400' as const },
  label: { fontSize: 13, fontWeight: '500' as const },
  caption: { fontSize: 12, fontWeight: '400' as const },
} as const;

// Minimum tap target, in points.
export const tapTarget = 44;

// Sizes that recur across screens.
export const sizes = { avatar: 52, avatarSmall: 40, avatarLarge: 96, fab: 56, control: 48 } as const;

// Browsers draw their own focus ring around an input; the field's border
// already says which one has focus, so the ring is turned off on the web.
export const inputReset: TextStyle =
  Platform.OS === 'web' ? ({ outlineStyle: 'none' } as unknown as TextStyle) : {};
