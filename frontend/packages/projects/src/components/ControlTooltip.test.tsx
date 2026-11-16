import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Box } from '@mui/material';
import { Composer } from './Composer';
import { ControlTooltip } from './ControlTooltip';
import { EffortPicker } from './EffortPicker';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(cleanup);

// Not by role: an open menu marks everything outside it aria-hidden, the tooltip included.
const tip = () => document.querySelector('[role="tooltip"]');
const hover = async (el: HTMLElement) => {
    fireEvent.mouseOver(el);
    await screen.findByRole('tooltip');
};
// Let any hover timers run, then assert the tooltip never came back.
const stillHidden = async () => {
    await act(async () => { await new Promise((r) => setTimeout(r, 400)); });
    expect(tip()).not.toBeInTheDocument();
};

describe('ControlTooltip on a picker', () => {
    const picker = () => render(
        <ControlTooltip title="Applies next turn">
            <Box><EffortPicker value="low" onChange={vi.fn()} /></Box>
        </ControlTooltip>,
    );

    it('shows on hover, and a click hides it while the options open uncovered', async () => {
        picker();
        const trigger = screen.getByRole('button', { name: 'effort.low' });
        await hover(trigger);
        expect(tip()).toHaveTextContent('Applies next turn');

        fireEvent.mouseDown(trigger);
        fireEvent.click(trigger);
        await waitFor(() => expect(tip()).not.toBeInTheDocument());
        expect(screen.getByRole('menuitem', { name: /effort.high/ })).toBeVisible();
    });

    it('stays hidden while the menu is open, even as the pointer moves over it and the trigger', async () => {
        picker();
        const trigger = screen.getByRole('button', { name: 'effort.low' });
        await hover(trigger);
        fireEvent.click(trigger);
        await waitFor(() => expect(tip()).not.toBeInTheDocument());

        fireEvent.mouseOver(screen.getByRole('menuitem', { name: /effort.high/ }));
        await stillHidden();
        fireEvent.mouseOver(trigger);
        await stillHidden();
    });

    it('hides when a keyboard press opens the menu', async () => {
        picker();
        const trigger = screen.getByRole('button', { name: 'effort.low' });
        fireEvent.keyDown(document.body, { key: 'Tab' });
        act(() => trigger.focus());
        fireEvent.focus(trigger);
        await screen.findByRole('tooltip');

        fireEvent.keyDown(trigger, { key: 'Enter' });
        fireEvent.click(trigger);
        await waitFor(() => expect(tip()).not.toBeInTheDocument());
        expect(screen.getByRole('menuitem', { name: /effort.high/ })).toBeVisible();
    });

    it('shows again on the next hover after the pointer has left', async () => {
        picker();
        const trigger = screen.getByRole('button', { name: 'effort.low' });
        await hover(trigger);
        fireEvent.click(trigger);
        await waitFor(() => expect(tip()).not.toBeInTheDocument());
        fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
        fireEvent.mouseLeave(trigger);
        await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument());

        await hover(trigger);
        expect(tip()).toHaveTextContent('Applies next turn');
    });
});

describe('ControlTooltip with a known menu state', () => {
    it('stays hidden while suppressed, whatever the pointer does', async () => {
        const view = render(<ControlTooltip title="Help" suppressed><button>Change</button></ControlTooltip>);
        fireEvent.mouseOver(screen.getByRole('button'));
        await stillHidden();
        view.rerender(<ControlTooltip title="Help"><button>Change</button></ControlTooltip>);
        await hover(screen.getByRole('button'));
    });
});

describe('Composer send options', () => {
    const open = () => {
        render(<Composer placeholder="Reply" streaming sendWhileStreaming sendModes onStop={vi.fn()} onSend={vi.fn()} />);
        fireEvent.change(screen.getByPlaceholderText('Reply'), { target: { value: 'change course' } });
        return screen.getByRole('button', { name: 'composer.sendOptions' });
    };

    it('shows its tooltip on hover and drops it once the menu is open', async () => {
        const trigger = open();
        await hover(trigger);
        expect(tip()).toHaveTextContent('composer.sendOptions');

        fireEvent.mouseDown(trigger);
        fireEvent.click(trigger);
        await waitFor(() => expect(tip()).not.toBeInTheDocument());
        expect(screen.getByText('composer.sendQueue')).toBeVisible();

        fireEvent.mouseOver(trigger);
        await stillHidden();
    });
});
