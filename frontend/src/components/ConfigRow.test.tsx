import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import ConfigRow from './ConfigRow';

const row = (
    <ConfigRow
        activeTab="baseUrl"
        onTabChange={() => undefined}
        tabs={[{ key: 'baseUrl', label: 'Base URL', content: <span>http://localhost:12580/tingly/claude_code</span>, actions: <button>copy</button> }]}
    />
);
const follows = (a: HTMLElement, b: HTMLElement) => !!(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);

describe('ConfigRow', () => {
    it('puts the label and its actions on one line and the full value under them in the phone shell', () => {
        render(<PhoneShellContext.Provider value>{row}</PhoneShellContext.Provider>);
        const value = screen.getByText(/^http:/);
        expect(follows(screen.getByRole('button', { name: 'copy' }), value)).toBe(true);
        expect(follows(screen.getByText('Base URL'), value)).toBe(true);
    });

    it('keeps label | value | actions on the desktop', () => {
        render(row);
        expect(follows(screen.getByText(/^http:/), screen.getByRole('button', { name: 'copy' }))).toBe(true);
    });
});
