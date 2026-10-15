import { formatBytes, formatTokens, timeAgo } from './format';
import { estimateTokens } from './tokens';

test('formatTokens shows concrete compact values', () => {
    expect(formatTokens(950)).toBe('950');
    expect(formatTokens(12_345)).toBe('12.3k');
    expect(formatTokens(100_000)).toBe('100k');
    expect(formatTokens(1_500_000)).toBe('1.5M');
});

test('formatBytes', () => {
    expect(formatBytes(78)).toBe('78 B');
    expect(formatBytes(2048)).toBe('2 KB');
});

test('estimateTokens matches the backend estimate', () => {
    // Same cases as internal/knowledge TestEstimateTokens.
    expect(estimateTokens('')).toBe(0);
    expect(estimateTokens('abcd')).toBe(1);
    expect(estimateTokens('abcde')).toBe(2);
    expect(estimateTokens('你好世界')).toBe(4);
});

test('timeAgo survives a locale tag Intl rejects', () => {
    const iso = new Date(Date.now() - 120_000).toISOString();
    expect(() => timeAgo(iso, 'en-US@posix')).not.toThrow();
    expect(timeAgo(iso, 'en-US')).toBe('2 minutes ago');
});
