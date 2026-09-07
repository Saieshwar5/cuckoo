/*
 * Syntax highlighting in greys.
 *
 * A code block is the loudest thing on a page, and the app's rule is that only
 * an error gets a colour. So the usual rainbow is replaced by one axis:
 * brightness. The values you care about — strings, numbers, the names of the
 * things being defined — are the brightest. Keywords and punctuation, which
 * you read past, recede. Comments recede furthest.
 *
 * Both halves of the site use these, so the snippet on the landing page and the
 * one in the quickstart are the same object.
 */

interface Rule {
  scope: string[];
  settings: { foreground?: string; fontStyle?: string };
}

function theme(
  name: string,
  type: 'dark' | 'light',
  ink: { bg: string; fg: string; loud: string; quiet: string; faint: string },
): Record<string, unknown> {
  const rules: Rule[] = [
    {
      scope: ['comment', 'punctuation.definition.comment', 'string.comment'],
      settings: { foreground: ink.faint, fontStyle: 'italic' },
    },
    {
      // The content: what a reader is actually looking for.
      scope: [
        'string',
        'string.quoted',
        'constant.numeric',
        'constant.language',
        'constant.character',
        'support.type.property-name',
        'meta.object-literal.key',
      ],
      settings: { foreground: ink.loud },
    },
    {
      // What is being defined or called.
      scope: [
        'entity.name.function',
        'support.function',
        'meta.function-call.generic',
        'entity.name.class',
        'entity.name.type',
        'support.class',
        'support.type',
      ],
      settings: { foreground: ink.loud, fontStyle: 'bold' },
    },
    {
      // Grammar: read past it.
      scope: [
        'keyword',
        'storage',
        'storage.type',
        'storage.modifier',
        'keyword.control',
        'keyword.operator',
        'variable.language',
        'entity.name.tag',
      ],
      settings: { foreground: ink.quiet },
    },
    {
      scope: ['punctuation', 'meta.brace', 'meta.delimiter'],
      settings: { foreground: ink.quiet },
    },
    {
      scope: ['variable', 'variable.other', 'variable.parameter', 'entity.other.attribute-name'],
      settings: { foreground: ink.fg },
    },
    {
      scope: ['invalid', 'invalid.illegal'],
      settings: { foreground: type === 'dark' ? '#FF5A50' : '#D0342C' },
    },
  ];

  return {
    name,
    type,
    colors: {
      'editor.background': ink.bg,
      'editor.foreground': ink.fg,
    },
    settings: rules,
  };
}

export const codeDark = theme('cuckoo-dark', 'dark', {
  bg: '#121212',
  fg: '#C9C9C9',
  loud: '#FFFFFF',
  quiet: '#8E8E8E',
  faint: '#6A6A6A',
});

export const codeLight = theme('cuckoo-light', 'light', {
  bg: '#F7F7F7',
  fg: '#3A3A3A',
  loud: '#000000',
  quiet: '#8A8A8A',
  faint: '#9A9A9A',
});
