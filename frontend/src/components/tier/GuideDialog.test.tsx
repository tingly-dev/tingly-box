import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { EntryGuideDialog } from './EntryGuideDialog';
import { TierGuideDialog } from './TierGuideDialog';

vi.mock('./StaticGraphViewer', () => ({ StaticGraphViewer: () => null }));
vi.mock('./GuideLanguageToggle', () => ({ GuideLanguageToggle: () => null }));

describe.each([
    ['entry guide', EntryGuideDialog],
    ['tier guide', TierGuideDialog],
] as const)('%s', (_name, Guide) => {
    it('uses the current close callback on Escape and removes the listener when closed', () => {
        const first = vi.fn();
        const second = vi.fn();
        const { rerender } = render(<Guide open onClose={first} />);

        rerender(<Guide open onClose={second} />);
        fireEvent.keyDown(document, { key: 'Escape' });
        expect(first).not.toHaveBeenCalled();
        expect(second).toHaveBeenCalledOnce();

        rerender(<Guide open={false} onClose={second} />);
        fireEvent.keyDown(document, { key: 'Escape' });
        expect(second).toHaveBeenCalledOnce();
    });
});
