import React from 'react';
import { Box, Switch, Tooltip, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';

export interface OneMContextSwitchProps {
    checked: boolean;
    onToggle: (next: boolean) => void;
    disabled?: boolean;
}

/**
 * OneMContextSwitch is the promoted per-rule control for the 1M context window
 * (the `context_1m` rule flag). It sits inline next to the request-model name so
 * the 1M decision lives with the model it widens, not buried in the flag
 * catalog. Enabling it makes the gateway add the context-1m beta upstream
 * (Claude Code / Anthropic) and widen the Codex model-catalog context window.
 * See .design/one-m-context.md.
 */
export const OneMContextSwitch: React.FC<OneMContextSwitchProps> = ({ checked, onToggle, disabled }) => {
    return (
        <Tooltip
            title="1M context window for this rule. Claude Code / Anthropic targets get the context-1m beta (and the [1m] model suffix in Quick Config); Codex targets get a widened catalog context_window. The routed model must support it."
            placement="top"
        >
            <Box
                component="span"
                sx={(theme) => ({
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 0.25,
                    pl: 0.5,
                    borderRadius: 1,
                    border: '1px solid',
                    borderColor: checked
                        ? alpha(theme.palette.primary.main, 0.5)
                        : 'transparent',
                    backgroundColor: checked
                        ? alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.16 : 0.08)
                        : 'transparent',
                })}
            >
                <Typography
                    component="span"
                    sx={(theme) => ({
                        fontSize: '0.66rem',
                        fontWeight: 700,
                        letterSpacing: '0.02em',
                        color: checked ? theme.palette.primary.main : theme.palette.text.disabled,
                    })}
                >
                    1M
                </Typography>
                <Switch
                    size="small"
                    checked={checked}
                    disabled={disabled}
                    onChange={(e) => onToggle(e.target.checked)}
                    inputProps={{ 'aria-label': 'Toggle 1M context window for this rule' }}
                />
            </Box>
        </Tooltip>
    );
};

export default OneMContextSwitch;
