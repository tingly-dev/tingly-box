import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import React, { useEffect, useMemo, useState } from 'react';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';

export interface FlagCatalogDialogProps {
    open: boolean;
    flags?: RuleFlags;
    registry?: FlagSpec[];
    loading?: boolean;
    onClose: () => void;
    onSave: (next: RuleFlags) => void;
}

const flagToBool = (flags: RuleFlags | undefined, key: string): boolean => {
    if (!flags) return false;
    switch (key) {
        case 'cursor_compat':
            return !!flags.cursorCompat;
        case 'cursor_compat_auto':
            return !!flags.cursorCompatAuto;
        case 'skip_usage':
            return !!flags.skipUsage;
        case 'use_max_completion_tokens':
            return !!flags.useMaxCompletionTokens;
        default:
            return false;
    }
};

const flagToString = (flags: RuleFlags | undefined, key: string): string => {
    if (!flags) return '';
    switch (key) {
        case 'custom_user_agent':
            return flags.customUserAgent || '';
        default:
            return '';
    }
};

const setBool = (flags: RuleFlags, key: string, value: boolean): RuleFlags => {
    switch (key) {
        case 'cursor_compat':
            return { ...flags, cursorCompat: value };
        case 'cursor_compat_auto':
            return { ...flags, cursorCompatAuto: value };
        case 'skip_usage':
            return { ...flags, skipUsage: value };
        case 'use_max_completion_tokens':
            return { ...flags, useMaxCompletionTokens: value };
        default:
            return flags;
    }
};

const setString = (flags: RuleFlags, key: string, value: string): RuleFlags => {
    switch (key) {
        case 'custom_user_agent':
            return { ...flags, customUserAgent: value };
        default:
            return flags;
    }
};

const CATEGORY_LABEL: Record<string, string> = {
    compatibility: 'Compatibility',
    request: 'Request',
    response: 'Response',
};

export const FlagCatalogDialog: React.FC<FlagCatalogDialogProps> = ({
    open,
    flags,
    registry,
    loading,
    onClose,
    onSave,
}) => {
    const [draft, setDraft] = useState<RuleFlags>(flags || {});

    // Reset the working copy whenever the dialog is (re-)opened with new flags.
    useEffect(() => {
        if (open) setDraft(flags ? { ...flags } : {});
    }, [open, flags]);

    const grouped = useMemo(() => {
        const groups = new Map<string, FlagSpec[]>();
        (registry || []).forEach((spec) => {
            const list = groups.get(spec.category) || [];
            list.push(spec);
            groups.set(spec.category, list);
        });
        return Array.from(groups.entries());
    }, [registry]);

    const handleToggle = (key: string, value: boolean) => {
        setDraft((d) => setBool(d, key, value));
    };

    const handleStringChange = (key: string, value: string) => {
        setDraft((d) => setString(d, key, value));
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                Rule Extensions
                <Typography variant="caption" component="div" color="text.secondary">
                    Pre-installed flags applied at the rule level.
                </Typography>
            </DialogTitle>
            <DialogContent dividers>
                {loading && (
                    <Typography variant="body2" color="text.secondary">
                        Loading flag catalog…
                    </Typography>
                )}
                {!loading && grouped.length === 0 && (
                    <Typography variant="body2" color="text.secondary">
                        No flags available.
                    </Typography>
                )}
                <Stack spacing={2}>
                    {grouped.map(([category, specs]) => (
                        <Box key={category}>
                            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1 }}>
                                <Typography variant="overline" sx={{ color: 'text.secondary', lineHeight: 1 }}>
                                    {CATEGORY_LABEL[category] || category}
                                </Typography>
                                <Divider sx={{ flexGrow: 1 }} />
                            </Stack>
                            <Stack spacing={1.5}>
                                {specs.map((spec) => {
                                    const enabled = spec.type === 'bool'
                                        ? flagToBool(draft, spec.key)
                                        : flagToString(draft, spec.key) !== '';
                                    return (
                                        <Box
                                            key={spec.key}
                                            sx={{
                                                p: 1,
                                                border: '1px solid',
                                                borderColor: enabled ? 'primary.light' : 'divider',
                                                borderRadius: 1,
                                                backgroundColor: enabled ? 'action.hover' : 'transparent',
                                            }}
                                        >
                                            <Stack direction="row" alignItems="center" spacing={1}>
                                                <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                                                    <Stack direction="row" alignItems="center" spacing={0.75}>
                                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                            {spec.label}
                                                        </Typography>
                                                        <Chip
                                                            size="small"
                                                            label={spec.key}
                                                            sx={{ height: 16, fontSize: '0.6rem' }}
                                                            variant="outlined"
                                                        />
                                                    </Stack>
                                                    <Typography variant="caption" color="text.secondary">
                                                        {spec.description}
                                                    </Typography>
                                                </Box>
                                                {spec.type === 'bool' && (
                                                    <Switch
                                                        size="small"
                                                        checked={flagToBool(draft, spec.key)}
                                                        onChange={(e) => handleToggle(spec.key, e.target.checked)}
                                                    />
                                                )}
                                            </Stack>
                                            {spec.type === 'string' && (
                                                <TextField
                                                    fullWidth
                                                    size="small"
                                                    placeholder={spec.placeholder}
                                                    value={flagToString(draft, spec.key)}
                                                    onChange={(e) => handleStringChange(spec.key, e.target.value)}
                                                    sx={{ mt: 1 }}
                                                />
                                            )}
                                        </Box>
                                    );
                                })}
                            </Stack>
                        </Box>
                    ))}
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} color="primary">
                    Cancel
                </Button>
                <Button onClick={() => onSave(draft)} color="primary" variant="contained">
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default FlagCatalogDialog;
