import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ChatNotices } from './ChatNotices';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
afterEach(cleanup);

function show(sandboxed: boolean, compact = false) {
    return render(<ChatNotices sandboxed={sandboxed} compact={compact} />);
}

describe('ChatNotices', () => {
    it('always shows the read-only capability line', () => {
        show(true);
        expect(screen.getByText('readonlyChats.capability')).toBeTruthy();
        expect(screen.queryByText('readonlyChats.unsandboxed')).toBeNull();
    });

    it('shows the persistent unsandboxed notice when there is no sandbox', () => {
        show(false);
        expect(screen.getByText('readonlyChats.unsandboxed')).toBeTruthy();
    });

    it('shows both as one compact line under the composer', () => {
        show(false, true);
        expect(screen.getByText('readonlyChats.canRead')).toBeTruthy();
        expect(screen.getByText('readonlyChats.unsandboxed')).toBeTruthy();
    });
});
