import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { PlacementMenu } from './PlacementMenu';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);

const show = (overview: 'side' | 'window' | 'top') => {
    render(<PlacementMenu overview={overview} thread="main" onOverview={vi.fn()} onThread={vi.fn()} overviewFloat={false} onOverviewFloat={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'placement.title' }));
};

describe('PlacementMenu', () => {
    it('says why a thread cannot sit beside or float over an Overview that is a top bar, under each option', () => {
        show('top');
        const disabled = screen.getAllByRole('menuitem').filter((el) => el.getAttribute('aria-disabled') === 'true');
        expect(disabled.map((el) => el.textContent)).toEqual([
            'placement.sideplacement.reasonTop', 'placement.floatplacement.reasonTop', 'placement.overviewFloatplacement.reasonTop',
        ]);
    });

    it('names what the float-from-Overview option needs when the Overview is not in the side pane', () => {
        show('window');
        expect(screen.getByText('placement.reasonSide')).toBeInTheDocument();
        expect(screen.queryByText('placement.reasonTop')).not.toBeInTheDocument();
    });

    it('offers every option without a reason when nothing is off', () => {
        show('side');
        expect(screen.queryByText('placement.reasonTop')).not.toBeInTheDocument();
        expect(screen.queryByText('placement.reasonSide')).not.toBeInTheDocument();
        expect(screen.getAllByRole('menuitem').some((el) => el.getAttribute('aria-disabled') === 'true')).toBe(false);
    });
});
