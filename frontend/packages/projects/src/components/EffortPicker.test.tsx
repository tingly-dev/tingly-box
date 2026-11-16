import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { Composer } from './Composer';
import { EffortPicker } from './EffortPicker';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

// A menu item's accessible name is its label and its hint line.
const item = (key: string) => `${key} ${key}Hint`;

afterEach(cleanup);

it('picks a level and can go back to the default', () => {
    const choose = vi.fn();
    render(<EffortPicker label="thread.effort" value="low" onChange={choose} />);
    expect(screen.getByRole('button', { name: 'thread.effort · effort.low' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'thread.effort · effort.low' }));
    fireEvent.click(screen.getByRole('menuitem', { name: item('effort.high') }));
    expect(choose).toHaveBeenCalledWith('high');

    fireEvent.click(screen.getByRole('button', { name: 'thread.effort · effort.low' }));
    fireEvent.click(screen.getByRole('menuitem', { name: item('effort.default') }));
    expect(choose).toHaveBeenCalledWith('');
});

it('shows an empty value as the default effort', () => {
    const choose = vi.fn();
    render(<EffortPicker value="" onChange={choose} />);
    expect(screen.getByRole('button', { name: 'effort.default' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'effort.default' }));
    expect(screen.getByRole('menuitem', { name: item('effort.default') })).toHaveClass('Mui-selected');
});

it('sits beside the model picker in the composer and reports the level', () => {
    const choose = vi.fn();
    render(<Composer effort="medium" onEffortChange={choose} onSend={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'effort.medium' }));
    fireEvent.click(screen.getByRole('menuitem', { name: item('effort.max') }));
    expect(choose).toHaveBeenCalledWith('max');
});


it('displays the inherited level and can restore project inheritance', () => {
    const choose = vi.fn();
    const view = render(<EffortPicker value="" defaultLabel="Project default · Medium" defaultDisplayLabel="Medium" onChange={choose} />);
    fireEvent.click(screen.getByRole('button', { name: 'Medium' }));
    expect(screen.getByRole('menuitem', { name: /Project default · Medium/ })).toHaveClass('Mui-selected');
    fireEvent.click(screen.getByRole('menuitem', { name: /effort.high/ }));
    expect(choose).toHaveBeenCalledWith('high');
    view.rerender(<EffortPicker value="high" defaultLabel="Project default · Medium" defaultDisplayLabel="Medium" onChange={choose} />);
    fireEvent.click(screen.getByRole('button', { name: 'effort.high' }));
    fireEvent.click(screen.getByRole('menuitem', { name: /Project default · Medium/ }));
    expect(choose).toHaveBeenLastCalledWith('');
});
