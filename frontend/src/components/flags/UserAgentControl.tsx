import { Check as IconCheck, KeyboardArrowDown as IconChevronDown, Circle as IconCircleFilled } from '@/components/icons';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    ListItemText,
    Menu,
    MenuItem,
    TextField,
    Tooltip,
} from '@mui/material';
import React, { useState } from 'react';

// USER_AGENT_NONE mirrors typ.UserAgentNone on the backend: the sentinel value
// that strips the outbound User-Agent header entirely (send no UA), distinct
// from an empty value which means "do not override".
export const USER_AGENT_NONE = 'none';

// Presets mirror typ.DefaultUserAgents() on the backend. Kept local (like
// RECORD_V2_MODES) so the control is self-contained; update both together.
//
// `name` is only a hint — the UI always surfaces the concrete `value` (the
// literal User-Agent string actually sent) so the choice is never ambiguous.
const UA_PRESETS = [
    { value: '', name: 'Default', description: "Don't override — keep the vendor/provider User-Agent" },
    { value: 'claude-cli/2.1.86 (external, cli)', name: 'Claude Code (CLI)', description: 'Impersonate the Claude Code CLI' },
    { value: 'codex_cli_rs/0.20.0', name: 'Codex CLI', description: 'Impersonate the Codex CLI' },
    { value: 'openclaw/1.0.0', name: 'OpenClaw', description: 'Impersonate OpenClaw' },
    { value: 'hermes-agent/1.0.0', name: 'Hermes', description: 'Impersonate the Hermes agent' },
    { value: 'OpenAI/Python 1.51.0', name: 'OpenAI Python SDK', description: 'Impersonate the OpenAI Python SDK' },
    { value: USER_AGENT_NONE, name: 'No User-Agent', description: 'Strip the User-Agent header — send no UA at all' },
] as const;

interface UserAgentControlProps {
    value: string;
    disabled?: boolean;
    onChange: (ua: string) => void;
}

const presetFor = (value: string) => UA_PRESETS.find(p => p.value === value);

// isLiteral marks values whose primary display IS the raw User-Agent string
// (i.e. real UAs). The two non-UA modes — "" (Default) and the `none` sentinel
// — have no real header to show, so they keep a descriptive label instead.
const isLiteral = (value: string) => value !== '' && value !== USER_AGENT_NONE;

// Primary text shown on the button / as the menu row's main line.
const primaryText = (value: string): string => {
    if (value === '') return 'Default';
    if (value === USER_AGENT_NONE) return 'No User-Agent';
    return value; // the concrete UA string
};

const UserAgentControl: React.FC<UserAgentControlProps> = ({ value, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [customOpen, setCustomOpen] = useState(false);
    const [customDraft, setCustomDraft] = useState('');

    const isActive = value !== '';
    const isCustom = isActive && !presetFor(value);

    const pick = (next: string) => {
        setAnchor(null);
        if (next !== value) onChange(next);
    };

    const openCustom = () => {
        setAnchor(null);
        setCustomDraft(isCustom ? value : '');
        setCustomOpen(true);
    };

    const applyCustom = () => {
        setCustomOpen(false);
        const next = customDraft.trim();
        if (next !== value) onChange(next);
    };

    return (
        <>
            <Tooltip
                title={`User-Agent: ${isActive ? value : "Default (don't override)"}`}
                placement="right"
                arrow
            >
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !disabled && setAnchor(e.currentTarget)}
                    disabled={disabled}
                    endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                    sx={{
                        minWidth: 110,
                        maxWidth: 260,
                        textTransform: 'none',
                        bgcolor: isActive ? 'primary.main' : 'transparent',
                        color: isActive ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isActive ? 600 : 400,
                        border: isActive ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isActive ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <IconCircleFilled sx={{ fontSize: 14, mr: '4px', flexShrink: 0 }} />
                    <Box component="span" sx={{ flexShrink: 0 }}>UA:&nbsp;</Box>
                    <Box
                        component="span"
                        sx={{
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            fontFamily: isLiteral(value) ? 'monospace' : undefined,
                        }}
                    >
                        {primaryText(value)}
                    </Box>
                </Button>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {UA_PRESETS.map((preset) => (
                    <MenuItem
                        key={preset.value || 'default'}
                        selected={preset.value === value}
                        onClick={() => pick(preset.value)}
                        title={preset.description}
                        sx={{ maxWidth: 360 }}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText
                                primary={primaryText(preset.value)}
                                // For real UAs the literal string is the primary line; the
                                // friendly name drops to a caption so nothing is hidden.
                                secondary={isLiteral(preset.value) ? preset.name : preset.description}
                                primaryTypographyProps={{
                                    variant: 'body2',
                                    fontFamily: isLiteral(preset.value) ? 'monospace' : undefined,
                                    sx: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
                                }}
                                secondaryTypographyProps={{ variant: 'caption' }}
                            />
                            {preset.value === value && <IconCheck sx={{ fontSize: 16, flexShrink: 0 }} />}
                        </Box>
                    </MenuItem>
                ))}
                <MenuItem onClick={openCustom} title="Enter a custom User-Agent string">
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                        <ListItemText
                            primary={isCustom ? value : 'Custom…'}
                            secondary={isCustom ? 'Custom value' : undefined}
                            primaryTypographyProps={{
                                variant: 'body2',
                                fontFamily: isCustom ? 'monospace' : undefined,
                                sx: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
                            }}
                            secondaryTypographyProps={{ variant: 'caption' }}
                        />
                        {isCustom && <IconCheck sx={{ fontSize: 16, flexShrink: 0 }} />}
                    </Box>
                </MenuItem>
            </Menu>
            <Dialog open={customOpen} onClose={() => setCustomOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>Custom User-Agent</DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        size="small"
                        placeholder="e.g. MyApp/1.0"
                        value={customDraft}
                        onChange={(e) => setCustomDraft(e.target.value)}
                        sx={{ mt: 1 }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCustomOpen(false)}>Cancel</Button>
                    <Button variant="contained" onClick={applyCustom}>Apply</Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default UserAgentControl;
