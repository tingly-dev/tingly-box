// A small labelled segmented control for "which variant of this value?"
// choices that sit next to a copyable value — Local / Docker host for a Base
// URL, npx / global for a start command. Each option shows its label (and an
// optional icon), so the current choice reads without hovering; the tooltip
// explains what it changes. This replaces the earlier pattern of bare icon
// buttons with a green check badge on the active one.
import { Box, ToggleButton, ToggleButtonGroup, Tooltip } from '@mui/material';
import React from 'react';
import { fontSizes } from '@/theme/fonts';

// Fits the widest option in use (icon + "Docker") with room to spare.
const OPTION_WIDTH = 84;
// Fixed, so a toggle without icons is as tall as one with them. The other small
// controls in the connection card (the Plugins strip) use the same height.
export const CONTROL_HEIGHT = 26;

export interface ChoiceOption<T extends string> {
    value: T;
    label: string;
    tooltip?: string;
    icon?: React.ReactNode;
}

interface ChoiceToggleProps<T extends string> {
    value: T;
    options: ChoiceOption<T>[];
    onChange: (value: T) => void;
    ariaLabel?: string;
    /** Width of every option when the default doesn't fit the longest label. */
    optionWidth?: number;
}

export function ChoiceToggle<T extends string>({ value, options, onChange, ariaLabel, optionWidth = OPTION_WIDTH }: ChoiceToggleProps<T>) {
    return (
        <ToggleButtonGroup
            exclusive
            size="small"
            value={value}
            aria-label={ariaLabel}
            onChange={(_, next: T | null) => next && onChange(next)}
            // Every option is the same width, so switches next to each other
            // (Local / Docker under NPX / Global) line up, and so do the copy
            // buttons beside them.
            sx={{ ml: 0.5, '& .MuiToggleButton-root': { width: optionWidth, height: CONTROL_HEIGHT, justifyContent: 'center', px: 0.75, py: 0, gap: 0.5, textTransform: 'none', fontSize: fontSizes.sm, lineHeight: 1.2 } }}
        >
            {options.map((option) => {
                const button = (
                    <ToggleButton key={option.value} value={option.value} aria-label={option.label}>
                        {option.icon && <Box sx={{ display: 'inline-flex', '& svg': { width: 16, height: 16 } }}>{option.icon}</Box>}
                        {option.label}
                    </ToggleButton>
                );
                return option.tooltip ? <Tooltip key={option.value} title={option.tooltip} arrow>{button}</Tooltip> : button;
            })}
        </ToggleButtonGroup>
    );
}

export default ChoiceToggle;
