import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import PageHeader from './PageHeader';
import UnifiedCard from './UnifiedCard';

describe('page heading semantics', () => {
    it('renders the shared page title as h1', () => {
        render(
            <PageHeader
                title="Agents"
                subtitle="Choose an agent."
                icon={<span data-testid="page-icon">A</span>}
            />,
        );

        expect(screen.getByRole('heading', { level: 1, name: 'Agents' })).toBeInTheDocument();
        expect(screen.getByTestId('page-icon').parentElement).toHaveAttribute('aria-hidden', 'true');
    });

    it('renders string card titles as h2 by default', () => {
        render(<UnifiedCard title="Policy Breakdown">Content</UnifiedCard>);

        expect(
            screen.getByRole('heading', { level: 2, name: 'Policy Breakdown' }),
        ).toBeInTheDocument();
    });

    it('supports a semantic h1 for composed card titles', () => {
        render(
            <UnifiedCard
                title={<span>Server Status</span>}
                titleHeadingLevel={1}
            >
                Content
            </UnifiedCard>,
        );

        expect(
            screen.getByRole('heading', { level: 1, name: 'Server Status' }),
        ).toBeInTheDocument();
    });
});
