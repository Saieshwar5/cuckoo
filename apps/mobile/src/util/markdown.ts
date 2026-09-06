// Markdown-lite: how the words in a bubble are drawn.
//
// Bold, italic, code, bullet and numbered lists, and links that are the
// link. Not headings, tables, images or quotes — those make a document,
// and a bubble is not one.
//
// Not [label](url) either. A worded link hides where it goes, and an agent
// claiming to be a bank is the first abuse to expect here; a link in text
// always shows the address it opens. An agent that wants a worded link uses
// a url button, which is visibly its own and carries an arrow.
//
// The hub stores what was sent, exactly. This is only the drawing, which is
// why an unclosed `**bo` of a reply still arriving stays literal until its
// closing marks land: a bubble must not flicker between styles while a
// model is still writing into it.

export interface Span {
  text: string;
  bold?: boolean;
  italic?: boolean;
  code?: boolean;
  // Where this span goes when tapped. The text is the address itself.
  link?: string;
}

export interface Line {
  spans: Span[];
  // A list item's marker: '•' for a bullet, '1.' for a number.
  // Undefined for an ordinary line, and for the blank line between
  // paragraphs, which has no spans either.
  marker?: string;
}

// A list marker needs whitespace after it, which is what separates '- one'
// from an unclosed *italic and '1. two' from a price.
const BULLET = /^\s*[-*+][ \t]+(?=\S)/;
const NUMBER = /^\s*(\d{1,3})[.)][ \t]+(?=\S)/;

// How deep styles may nest before the rest is left as text. Four covers
// anything written on purpose.
const MAX_DEPTH = 4;

interface Mark {
  open: string;
  close: string;
  key: 'bold' | 'italic' | 'code';
  // Underscores must sit at the edge of a word, or snake_case_names would
  // come out italic halfway through.
  word?: boolean;
}

// Longest opener first: ** must be tried before *.
const MARKS: Mark[] = [
  { open: '`', close: '`', key: 'code' },
  { open: '**', close: '**', key: 'bold' },
  { open: '*', close: '*', key: 'italic' },
  { open: '_', close: '_', key: 'italic', word: true },
];

// Everything that is not a word character. Deliberately not \w, which is
// ASCII: a Telugu letter is a word character here, as it should be.
const PUNCTUATION = ' \t!"#$%&\'()*+,-./:;<=>?@[\\]^`{|}~';

function isWord(ch: string | undefined): boolean {
  return ch !== undefined && !PUNCTUATION.includes(ch);
}

// parse turns a message's text into lines of styled spans.
export function parse(text: string): Line[] {
  const lines: Line[] = [];
  let gap = false;
  for (const raw of text.split('\n')) {
    if (raw.trim() === '') {
      // Runs of empty lines are one gap: a model that double-spaces its
      // paragraphs should not push the bubble open.
      if (lines.length > 0) gap = true;
      continue;
    }
    if (gap) {
      lines.push({ spans: [] });
      gap = false;
    }
    const number = NUMBER.exec(raw);
    if (number) {
      lines.push({ spans: inline(raw.slice(number[0].length), 0), marker: `${number[1]}.` });
      continue;
    }
    const bullet = BULLET.exec(raw);
    if (bullet) {
      lines.push({ spans: inline(raw.slice(bullet[0].length), 0), marker: '•' });
      continue;
    }
    lines.push({ spans: inline(raw, 0) });
  }
  return lines;
}

// plain is the same text with its marks removed, on one line: what a chat
// list row and a reply quote show, where there is no room to draw anything.
export function plain(text: string): string {
  return parse(text)
    .map((line) => line.spans.map((s) => s.text).join(''))
    .join(' ')
    .replace(/\s+/g, ' ')
    .trim();
}

// inline walks one line, emitting a span wherever a mark opens and closes
// and plain text everywhere else.
function inline(src: string, depth: number): Span[] {
  const spans: Span[] = [];
  let literal = '';
  let i = 0;
  const flush = () => {
    if (literal) spans.push(...autolink(literal));
    literal = '';
  };
  while (i < src.length) {
    const styled = openAt(src, i, depth);
    if (styled) {
      flush();
      spans.push(...styled.spans);
      i = styled.next;
      continue;
    }
    literal += src[i];
    i += 1;
  }
  flush();
  return spans;
}

// openAt returns the span a mark starting here produces, or null when
// nothing opens here or nothing closes it — which is what leaves a reply
// still being written alone.
function openAt(src: string, i: number, depth: number): { spans: Span[]; next: number } | null {
  for (const mark of MARKS) {
    if (!src.startsWith(mark.open, i)) continue;
    if (mark.word && isWord(src[i - 1])) continue;
    const from = i + mark.open.length;
    const close = findClose(src, from, mark);
    if (close < 0 || close === from) continue;
    const inner = src.slice(from, close);
    // Code is literal: marks inside it are the point of writing it.
    const spans =
      mark.key === 'code'
        ? [{ text: inner, code: true }]
        : (depth < MAX_DEPTH ? inline(inner, depth + 1) : [{ text: inner }]).map((s) => ({
            ...s,
            [mark.key]: true,
          }));
    return { spans, next: close + mark.close.length };
  }
  return null;
}

function findClose(src: string, from: number, mark: Mark): number {
  for (let j = from; j <= src.length - mark.close.length; j += 1) {
    if (!src.startsWith(mark.close, j)) continue;
    if (mark.word && isWord(src[j + mark.close.length])) continue;
    return j;
  }
  return -1;
}

// autolink makes a bare address tappable. Brackets and quotes end it, so a
// link written inside [](…) or "…" keeps its punctuation out of the target.
function autolink(text: string): Span[] {
  const pattern = /https?:\/\/[^\s<>()[\]"'`]+/g;
  const spans: Span[] = [];
  let last = 0;
  for (let m = pattern.exec(text); m; m = pattern.exec(text)) {
    const url = m[0].replace(/[.,;:!?]+$/, '');
    if (!url.includes('.')) continue;
    if (m.index > last) spans.push({ text: text.slice(last, m.index) });
    spans.push({ text: url, link: url });
    last = m.index + url.length;
    pattern.lastIndex = last;
  }
  if (last < text.length) spans.push({ text: text.slice(last) });
  return spans;
}
