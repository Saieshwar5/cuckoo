import React from 'react';
import { Platform, StyleSheet, Text, View, type TextStyle } from 'react-native';

import { spacing } from '@/theme';
import { parse, type Span } from '@/util/markdown';

interface Props {
  text: string;
  // The bubble's own text style: its colour, size and line height.
  style: TextStyle | TextStyle[];
  // Drawn at the very end, after the last word: the caret of a reply still
  // being written.
  trailing?: React.ReactNode;
  onLink: (url: string) => void;
}

// RichText draws what a message says: bold, italic, code, lists, and
// addresses that can be tapped. What counts as which is in util/markdown;
// this is only the drawing of it.
//
// Nothing here is coloured. The palette is black, white and grey, so a link
// is underlined rather than blue — which is also the honest signal, since a
// colour would say "trust me" and an underline only says "this leaves".
export function RichText({ text, style, trailing, onLink }: Props) {
  const lines = parse(text);
  if (lines.length === 0) return trailing ? <Text style={style}>{trailing}</Text> : null;

  return (
    <View>
      {lines.map((line, i) => {
        const last = i === lines.length - 1;
        const tail = last ? trailing : null;
        if (line.spans.length === 0) return <View key={i} style={styles.gap} />;
        const body = (
          <Text style={style}>
            {line.spans.map((span, j) => (
              <Piece key={j} span={span} onLink={onLink} />
            ))}
            {tail}
          </Text>
        );
        if (!line.marker) return <React.Fragment key={i}>{body}</React.Fragment>;
        return (
          <View key={i} style={styles.item}>
            <Text style={[style, styles.marker]}>{line.marker}</Text>
            <View style={styles.itemBody}>{body}</View>
          </View>
        );
      })}
    </View>
  );
}

function Piece({ span, onLink }: { span: Span; onLink: (url: string) => void }) {
  const style: TextStyle = {};
  if (span.bold) style.fontWeight = '700';
  if (span.italic) style.fontStyle = 'italic';
  if (span.link) return <Link span={span} style={style} onLink={onLink} />;
  if (span.code) return <Text style={[style, styles.code]}>{span.text}</Text>;
  return <Text style={style}>{span.text}</Text>;
}

function Link({ span, style, onLink }: { span: Span; style: TextStyle; onLink: (url: string) => void }) {
  return (
    <Text
      style={[style, styles.link]}
      accessibilityRole="link"
      testID={`link-${span.link}`}
      onPress={() => onLink(span.link as string)}
    >
      {span.text}
    </Text>
  );
}

// A line that reads as one on any device: the platform's own monospace.
const mono = Platform.select({ ios: 'Menlo', android: 'monospace', default: 'monospace' });

const styles = StyleSheet.create({
  // The blank line between paragraphs. Shorter than a line of text: it is
  // a breath, not an empty row.
  gap: { height: spacing.xs + 2 },
  item: { flexDirection: 'row' },
  // Wide enough for '10.' so the text of every item in a list starts in the
  // same place.
  marker: { width: 22 },
  itemBody: { flex: 1 },
  // Grey at a quarter strength sits on both the light bubble and the dark
  // one without either palette having to name a colour for it.
  code: { fontFamily: mono, fontSize: 14, backgroundColor: 'rgba(128, 128, 128, 0.22)' },
  link: { textDecorationLine: 'underline' },
});
