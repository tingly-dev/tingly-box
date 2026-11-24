import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CardGrid } from './CardGrid';

describe('CardGrid', () => {
    it('can switch virtualization on and off after mounting', () => {
        const cards = [<div key="first">First card</div>, <div key="second">Second card</div>];
        const grid = (virtualized: boolean) => (
            <CardGrid virtualized={virtualized} spacing={0} itemHeight={100} containerHeight={100} overscan={0}>
                {cards}
            </CardGrid>
        );

        const { rerender } = render(grid(false));
        expect(screen.getByText('Second card')).toBeInTheDocument();

        rerender(grid(true));
        expect(screen.getByText('First card')).toBeInTheDocument();
        expect(screen.queryByText('Second card')).not.toBeInTheDocument();

        rerender(grid(false));
        expect(screen.getByText('Second card')).toBeInTheDocument();
    });
});
