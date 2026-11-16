import { describe, expect, it } from 'vitest';
import { compactDefaultWarning } from './CodexQuickConfig';

const text = { low: 'low {remain} {min}', limited: 'limited {effective} {applied}' };

describe('compactDefaultWarning', () => {
    it('is quiet for the defaults', () => {
        expect(compactDefaultWarning({}, text)).toBeNull();
    });

    it('warns when too few tokens would remain', () => {
        const w = compactDefaultWarning({ effective_context_window_percent: '95', auto_compact_percent: '85' }, text);
        expect(w).toEqual({ severity: 'warning', message: 'low 30,000 36,000' });
    });

    it('says when the usable window cuts the value down', () => {
        const w = compactDefaultWarning({ auto_compact_percent: '90' }, text);
        expect(w).toEqual({ severity: 'info', message: 'limited 82 82' });
    });

    it('does not cap at Codex\'s 90%: 95% under a 100% window only warns about tokens left', () => {
        const w = compactDefaultWarning({ effective_context_window_percent: '100', auto_compact_percent: '95' }, text);
        expect(w).toEqual({ severity: 'warning', message: 'low 10,000 36,000' });
    });

    it('ignores unparseable input (falls back to defaults)', () => {
        expect(compactDefaultWarning({ auto_compact_percent: 'abc' }, text)).toBeNull();
    });
});
