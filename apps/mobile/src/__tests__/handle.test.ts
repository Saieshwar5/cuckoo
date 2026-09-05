import { isValidHandle, suggestHandle } from '@/util/handle';

describe('handles', () => {
  it('suggests one from a name and never suggests an invalid one', () => {
    expect(suggestHandle('My Assistant')).toBe('my-assistant');
    expect(suggestHandle('  SBI Support!! ')).toBe('sbi-support');
    expect(suggestHandle('Café Résumé')).toBe('cafe-resume');
    expect(suggestHandle('--weird__name--')).toBe('weird__name');
    expect(suggestHandle('Ab')).toBe('ab-agent');
    expect(suggestHandle('!!!')).toBe('');
    expect(suggestHandle('a'.repeat(50))).toHaveLength(32);
    for (const name of ['My Assistant', 'Ab', 'Café', 'x'.repeat(40), '9lives']) {
      const h = suggestHandle(name);
      if (h) expect(isValidHandle(h)).toBe(true);
    }
  });

  it('validates the hub rule', () => {
    expect(isValidHandle('echo')).toBe(true);
    expect(isValidHandle('a-b_c9')).toBe(true);
    expect(isValidHandle('ab')).toBe(false);
    expect(isValidHandle('-abc')).toBe(false);
    expect(isValidHandle('Echo')).toBe(false);
    expect(isValidHandle('a'.repeat(33))).toBe(false);
  });
});
