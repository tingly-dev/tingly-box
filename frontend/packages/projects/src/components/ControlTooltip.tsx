import { cloneElement, useRef, useState, type ReactElement, type SyntheticEvent } from 'react';
import { Tooltip, type TooltipProps } from '@mui/material';

type Handler = (e: SyntheticEvent) => void;
interface TriggerProps {
    onMouseOver?: Handler;
    onFocus?: Handler;
    onMouseLeave?: Handler;
    onBlur?: (e?: SyntheticEvent) => void;
    onMouseDown?: Handler;
    onClick?: Handler;
    onKeyDown?: (e: React.KeyboardEvent) => void;
}

interface Props extends Omit<TooltipProps, 'open' | 'onOpen' | 'onClose' | 'children'> {
    children: ReactElement<TriggerProps>;
    /** The control's menu or popover is open: the tooltip stays hidden whatever the pointer does. */
    suppressed?: boolean;
}

// Keys that open a menu from a focused trigger (a Select opens on keydown, a button on click).
const OPENING_KEYS = new Set(['Enter', ' ', 'ArrowDown', 'ArrowUp']);

// A menu rendered inside the trigger's React subtree still bubbles its events here.
// MUI inputs call onBlur without an event when they become disabled, hence the optional.
const fromTrigger = (e?: SyntheticEvent) => !e || e.currentTarget.contains(e.target as Node);

/**
 * A tooltip for a composer control that opens a menu. It shows on hover and
 * keyboard focus like any tooltip, but a press (or Enter/Space/arrow) on the
 * trigger dismisses it and keeps it away until the pointer or focus has left
 * the trigger, so it never sits on top of the options it sits beside.
 */
export function ControlTooltip({ children, suppressed = false, ...tooltip }: Props) {
    const [shown, setShown] = useState(false);
    const [dismissed, setDismissed] = useState(false);
    const overTrigger = useRef(true);
    const own = children.props;

    const rearm = (e?: SyntheticEvent) => {
        if (!fromTrigger(e)) return;
        setDismissed(false);
        setShown(false);
    };
    const dismiss = (e: SyntheticEvent) => {
        if (fromTrigger(e)) setDismissed(true);
    };
    const track = (e: SyntheticEvent) => { overTrigger.current = fromTrigger(e); };
    const open = () => { if (overTrigger.current) setShown(true); };

    return (
        <Tooltip
            {...tooltip}
            open={shown && !dismissed && !suppressed}
            onOpen={open}
            onClose={() => setShown(false)}
        >
            {cloneElement(children, {
                onMouseOver: (e) => { own.onMouseOver?.(e); track(e); },
                onFocus: (e) => { own.onFocus?.(e); track(e); },
                onMouseLeave: (e) => { own.onMouseLeave?.(e); rearm(e); },
                onBlur: (e) => { own.onBlur?.(e); rearm(e); },
                onMouseDown: (e) => { own.onMouseDown?.(e); dismiss(e); },
                onClick: (e) => { own.onClick?.(e); dismiss(e); },
                onKeyDown: (e) => { own.onKeyDown?.(e); if (OPENING_KEYS.has(e.key)) dismiss(e); },
            })}
        </Tooltip>
    );
}
