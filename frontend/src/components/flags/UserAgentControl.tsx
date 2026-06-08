import { IconCheck, IconChevronDown, IconCircleFilled } from '@tabler/icons-react';
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
const UA_PRESETS = [
    { value: '', label: 'Default', description: "Don't override — keep the vendor/provider User-Agent" },
    { value: 'claude-cli/2.1.86 (external, cli)', label: 'Claude Code (CLI)', description: 'Impersonate the Claude Code CLI' },
    { value: 'codex_cli_rs/0.20.0', label: 'Codex CLI', description: 'Impersonate the Codex CLI' },
    { value: 'openclaw/1.0.0', label: 'OpenClaw', description: 'Impersonate OpenClaw' },
    { value: 'hermes-agent/1.0.0', label: 'Hermes', description: 'Impersonate the Hermes agent' },
    { value: 'OpenAI/Python 1.51.0', label: 'OpenAI Python SDK', description: 'Impersonate the OpenAI Python SDK' },
    { value: USER_AGENT_NONE, label: 'None (no User-Agent)', description: 'Strip the User-Agent header — send no UA at all' },
] as const;

interface UserAgentControlProps {
    value: string;
    disabled?: boolean;
    onChange: (ua: string) => void;
}

const presetFor = (value: string) => UA_PRESETS.find(p => p.value === value);

const labelFor = (value: string): string => {
    if (value === '') return 'Default';
    return presetFor(value)?.label ?? 'Custom';
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
                    endIcon={<IconChevronDown size={18} />}
                    sx={{
                        minWidth: 110,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: isActive ? 'primary.main' : 'transparent',
                        color: isActive ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isActive ? 600 : 400,
                        border: isActive ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isActive ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <IconCircleFilled size={14} style={{ marginRight: '4px' }} />
                    UA: {labelFor(value)}
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
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={preset.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {preset.value === value && <IconCheck size={16} />}
                        </Box>
                    </MenuItem>
                ))}
                <MenuItem onClick={openCustom} title="Enter a custom User-Agent string">
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                        <ListItemText primary="Custom…" primaryTypographyProps={{ variant: 'body2' }} />
                        {isCustom && <IconCheck size={16} />}
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
