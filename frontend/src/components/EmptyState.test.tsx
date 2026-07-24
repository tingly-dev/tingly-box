import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import EmptyState from './EmptyState';

describe('EmptyState', () => {
    it('renders an informational state without inventing an action', () => {
        render(
            <EmptyState
                title="No Virtual Models Available"
                description="Virtual models are seeded at server startup."
            />,
        );

        expect(
            screen.getByRole('heading', {
                level: 3,
                name: 'No Virtual Models Available',
            }),
        ).toBeInTheDocument();
        expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });

    it('renders a caller-owned recovery action', () => {
        const onClick = vi.fn();

        render(
            <EmptyState
                title="No providers configured"
                primaryAction={{ label: 'Get started', onClick }}
            />,
        );

        fireEvent.click(screen.getByRole('button', { name: 'Get started' }));
        expect(onClick).toHaveBeenCalledOnce();
    });
});
