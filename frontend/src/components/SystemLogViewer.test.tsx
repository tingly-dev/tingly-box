import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SystemLogViewer from './SystemLogViewer';

vi.mock('@/hooks/useAutoRefresh', () => ({ useAutoRefresh: vi.fn() }));

describe('SystemLogViewer diagnostics', () => {
    it('shows API failures instead of an unexplained empty table', async () => {
        render(<SystemLogViewer getLogs={vi.fn().mockRejectedValue(new Error('HTTP 500'))} />);
        expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 500');
    });

    it('keeps the correct detail open when new logs arrive and searches fields', async () => {
        const older = { time: '2026-10-21T00:00:00Z', level: 'error', message: 'old request failed', fields: { request_id: 'r-old', error: 'provider overload' } };
        const newer = { time: '2026-10-21T00:00:01Z', level: 'info', message: 'new request succeeded', fields: { request_id: 'r-new' } };
        const getLogs = vi.fn().mockResolvedValueOnce({ total: 1, logs: [older] }).mockResolvedValue({ total: 2, logs: [newer, older] });
        render(<SystemLogViewer getLogs={getLogs} />);
        fireEvent.click(await screen.findByText('old request failed'));
        expect(await screen.findByText('ERROR: provider overload')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Refresh logs' }));
        await screen.findByText('new request succeeded');
        expect(screen.getByText('ERROR: provider overload')).toBeInTheDocument();
        fireEvent.change(screen.getByLabelText('Search logs'), { target: { value: 'provider overload' } });
        await waitFor(() => expect(screen.queryByText('new request succeeded')).not.toBeInTheDocument());
        expect(screen.getAllByText('old request failed').length).toBeGreaterThan(0);
    });
});
