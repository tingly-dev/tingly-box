import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AgentStatusRow, { AgentLogButton, formatErrorRate, formatRelativeTime } from './AgentStatusRow';

// No i18n instance in tests: t() returns the key, which is what we match on.
const base = {
    connected: true,
    configStatus: null,
};

describe('AgentStatusRow', () => {
    it('claims nothing until the first answer', () => {
        render(<AgentStatusRow {...base} observation={null} />);
        expect(screen.queryByTestId('agent-status-summary')).toBeNull();
        // text only: buttons live in the page header
        expect(screen.queryAllByRole('button')).toHaveLength(0);
    });

    it('says connected with the last request, today and error rate', () => {
        render(<AgentStatusRow {...base} observation={{ lastRequestAt: new Date().toISOString(), requestsToday: 200, errorsToday: 1 }} />);
        const text = screen.getByTestId('agent-status-summary').textContent ?? '';
        expect(text).toContain('agentStatus.connected');
        expect(text).toContain('agentStatus.lastRequest');
        expect(text).toContain('agentStatus.today');
        expect(text).toContain('agentStatus.errorRate');
    });

    it('asks to finish the setup when nothing has arrived and it is not connected', () => {
        render(<AgentStatusRow {...base} connected={false} observation={{ lastRequestAt: null, requestsToday: 0, errorsToday: 0 }} />);
        expect(screen.getByTestId('agent-status-summary').textContent).toBe('agentStatus.noRequestsFinishSetup');
    });

    it('notes an out-of-date config without a button of its own', () => {
        render(
            <AgentStatusRow
                {...base}
                observation={{ lastRequestAt: null, requestsToday: 0, errorsToday: 0 }}
                configStatus={{ state: 'outdated', differences: [{ key: 'a', applied: '1', expected: '2' }] } as never}
            />,
        );
        expect(screen.getByText('agentStatus.stale')).toBeInTheDocument();
        expect(screen.queryAllByRole('button')).toHaveLength(0);
    });
});

describe('AgentLogButton', () => {
    it('opens the log', () => {
        const onOpenLog = vi.fn();
        render(<AgentLogButton onOpenLog={onOpenLog} />);
        fireEvent.click(screen.getByRole('button', { name: 'agentStatus.log' }));
        expect(onOpenLog).toHaveBeenCalledOnce();
    });
});

describe('formatters', () => {
    it('formats the error rate', () => {
        expect(formatErrorRate(0, 0)).toBeNull();
        expect(formatErrorRate(200, 0)).toBe('0%');
        expect(formatErrorRate(200, 1)).toBe('0.5%');
        expect(formatErrorRate(10, 3)).toBe('30%');
    });

    it('formats relative time', () => {
        const now = Date.parse('2026-10-11T12:00:00Z');
        expect(formatRelativeTime('2026-10-11T11:58:00Z', 'en', now)).toBe('2 minutes ago');
        expect(formatRelativeTime('nope', 'en', now)).toBe('');
    });
});
