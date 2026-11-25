import { act, render } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { composeTitle, setTitleAttention, truncatePart, useDocumentTitle } from './documentTitle';

const Probe = ({ parts, priority }: { parts: (string | null)[]; priority?: number }) => {
  useDocumentTitle(parts, priority);
  return null;
};

describe('document title', () => {
  afterEach(() => setTitleAttention(false));

  it('composes focus, product and site', () => {
    expect(composeTitle(['#12 Fix', 'tb-ui'], { product: 'Tingly Box', site: 'a:1' })).toBe('#12 Fix · tb-ui · Tingly Box · a:1');
    expect(composeTitle([], { product: 'Tingly Box', attention: true })).toBe('● Tingly Box');
  });

  it('truncates long parts', () => {
    expect(Array.from(truncatePart('x'.repeat(60)))).toHaveLength(40);
    expect(truncatePart('short')).toBe('short');
  });

  it('prefers the higher priority and falls back when it unmounts', () => {
    document.title = 'Tingly Box';
    const shell = render(<Probe parts={['Credentials']} priority={0} />);
    expect(document.title.startsWith('Credentials · ')).toBe(true);
    const page = render(<Probe parts={['#3 Thing', null, 'proj']} priority={1} />);
    expect(document.title.startsWith('#3 Thing · proj · ')).toBe(true);
    page.unmount();
    expect(document.title.startsWith('Credentials · ')).toBe(true);
    act(() => setTitleAttention(true));
    expect(document.title.startsWith('● Credentials')).toBe(true);
    shell.unmount();
  });
});
