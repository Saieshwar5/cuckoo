import { parse, plain, type Span } from '@/util/markdown';

// Spans as "text" with the styles that apply, so a case reads as what the
// bubble shows rather than as a tree.
const shown = (spans: Span[]) =>
  spans.map((s) => {
    const marks = [s.bold && 'b', s.italic && 'i', s.code && 'c', s.link && `→${s.link}`].filter(Boolean);
    return marks.length ? `${s.text}[${marks.join('')}]` : s.text;
  });

const oneLine = (src: string) => shown(parse(src)[0]?.spans ?? []);

describe('markdown-lite', () => {
  it('draws bold, italic and code', () => {
    expect(oneLine('a **bold** b')).toEqual(['a ', 'bold[b]', ' b']);
    expect(oneLine('a *soft* b')).toEqual(['a ', 'soft[i]', ' b']);
    expect(oneLine('a _soft_ b')).toEqual(['a ', 'soft[i]', ' b']);
    expect(oneLine('run `make ci` now')).toEqual(['run ', 'make ci[c]', ' now']);
  });

  it('nests, and leaves code alone', () => {
    expect(oneLine('**bold _and_ more**')).toEqual(['bold [b]', 'and[bi]', ' more[b]']);
    expect(oneLine('`**not bold**`')).toEqual(['**not bold**[c]']);
  });

  // A reply still arriving must not flicker between styles as its closing
  // marks land, so an unclosed mark is text.
  it('leaves an unfinished mark as it is', () => {
    expect(oneLine('Your order **is')).toEqual(['Your order **is']);
    expect(oneLine('Your order **is on**')).toEqual(['Your order ', 'is on[b]']);
    expect(oneLine('2 * 3 = 6')).toEqual(['2 * 3 = 6']);
    expect(oneLine('****')).toEqual(['****']);
  });

  it('leaves a name with underscores alone', () => {
    expect(oneLine('call get_user_name first')).toEqual(['call get_user_name first']);
  });

  it('makes lists', () => {
    const lines = parse('Order 4412:\n- Rider: Ravi\n* ETA: 12 min\n\n1. Pay\n2) Collect');
    expect(lines.map((l) => l.marker)).toEqual([undefined, '•', '•', undefined, '1.', '2.']);
    expect(shown(lines[1]?.spans ?? [])).toEqual(['Rider: Ravi']);
    // The blank line between the two lists is one gap with nothing in it.
    expect(lines[3]?.spans).toEqual([]);
  });

  it('collapses a run of blank lines and ignores leading ones', () => {
    expect(parse('\n\na\n\n\n\nb').map((l) => l.spans.length)).toEqual([1, 0, 1]);
  });

  it('makes a bare address tappable, and only the address', () => {
    expect(oneLine('see https://swiggy.com/t/4412 now')).toEqual([
      'see ',
      'https://swiggy.com/t/4412[→https://swiggy.com/t/4412]',
      ' now',
    ]);
    expect(oneLine('open https://swiggy.com/t/4412.')).toEqual([
      'open ',
      'https://swiggy.com/t/4412[→https://swiggy.com/t/4412]',
      '.',
    ]);
    expect(oneLine('mail me at a@b.in')).toEqual(['mail me at a@b.in']);
  });

  // A worded link is not a thing an agent gets: the address is always shown.
  it('does not follow a worded link, but still shows its address', () => {
    expect(oneLine('[Track order](https://swiggy.com/t/4412)')).toEqual([
      '[Track order](',
      'https://swiggy.com/t/4412[→https://swiggy.com/t/4412]',
      ')',
    ]);
  });

  it('keeps other scripts whole', () => {
    expect(oneLine('**మీ ఆర్డర్** వస్తోంది')).toEqual(['మీ ఆర్డర్[b]', ' వస్తోంది']);
  });

  it('strips marks for a preview', () => {
    expect(plain('**Order 4412**\n- Rider: Ravi\n- ETA: 12 min')).toBe('Order 4412 Rider: Ravi ETA: 12 min');
    expect(plain('')).toBe('');
  });
});
