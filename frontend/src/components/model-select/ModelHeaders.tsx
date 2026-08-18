import { Extension as ExtensionIcon } from '@/components/icons';
import {
    Alert,
    Box,
    Button,
    CircularProgress,
    IconButton,
    Popover,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';
import HeadersEditor from '@/components/flags/HeadersEditor';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';

// canEditModelHeaders gates the per-model Custom Headers UI to api_key
// providers — the only auth type the extra_headers release covers (the
// backend rejects and the transport no-ops on everything else).
export const canEditModelHeaders = (provider: Provider): boolean =>
    (provider.auth_type ?? 'api_key') === 'api_key';

interface ModelHeadersTriggerProps {
    onOpen: (anchor: HTMLElement) => void;
}

// The control-bar entry that opens the per-model Custom Headers popover.
export const ModelHeadersTrigger: React.FC<ModelHeadersTriggerProps> = ({ onOpen }) => (
    <Tooltip title="Custom headers for this model">
        <IconButton
            size="small"
            onClick={(e) => onOpen(e.currentTarget)}
            sx={{
                p: 0.3,
                color: 'text.secondary',
                '&:hover': { backgroundColor: 'action.hover', color: 'primary.main' },
            }}
        >
            <ExtensionIcon sx={{ fontSize: 14 }} />
        </IconButton>
    </Tooltip>
);

interface ModelHeadersBadgeProps {
    /** Configured header names — shown in the tooltip so the concrete values are visible without opening. */
    names: string[];
    onOpen: (anchor: HTMLElement) => void;
}

// Always-visible marker (bottom-left) for models carrying header overrides,
// so overrides are discoverable while scanning the list, not only on hover.
export const ModelHeadersBadge: React.FC<ModelHeadersBadgeProps> = ({ names, onOpen }) => (
    <Tooltip title={`Custom headers: ${names.join(', ')}`} arrow>
        <Box
            onClick={(e) => { e.stopPropagation(); onOpen(e.currentTarget); }}
            sx={{
                position: 'absolute',
                bottom: 2,
                left: 4,
                display: 'flex',
                alignItems: 'center',
                color: 'primary.main',
                cursor: 'pointer',
                zIndex: 5,
            }}
        >
            <ExtensionIcon sx={{ fontSize: 12 }} />
        </Box>
    </Tooltip>
);

export interface ModelHeadersPopoverProps {
    provider: Provider;
    model: string;
    anchorEl: HTMLElement | null;
    /** Current override map for this model (drives the editor's initial rows). */
    initial?: Record<string, string>;
    onClose: () => void;
    /** Called with the saved map (undefined = override cleared) after a successful save. */
    onSaved: (next: Record<string, string> | undefined) => void;
}

/**
 * ModelHeadersPopover — inline editor for one model's extra_headers override.
 * Saves via a read-modify-write of the provider's model_flags map: the
 * provider is re-fetched first so concurrent edits to sibling models in the
 * same session are not clobbered.
 */
export const ModelHeadersPopover: React.FC<ModelHeadersPopoverProps> = ({
    provider,
    model,
    anchorEl,
    initial,
    onClose,
    onSaved,
}) => {
    const [draft, setDraft] = useState<Record<string, string> | undefined>(initial);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleSave = async () => {
        setSaving(true);
        setError(null);
        try {
            const fresh = await api.getProvider(provider.uuid);
            const all: Record<string, { extra_headers?: Record<string, string> }> = {
                ...((fresh?.data as Provider | undefined)?.model_flags ?? {}),
            };
            if (draft && Object.keys(draft).length > 0) {
                all[model] = { extra_headers: draft };
            } else {
                delete all[model];
            }
            const result = await api.updateProvider(provider.uuid, { model_flags: all });
            if (result.success) {
                onSaved(draft && Object.keys(draft).length > 0 ? draft : undefined);
                onClose();
            } else {
                setError(result.error || 'Failed to save');
            }
        } catch (e: any) {
            setError(e?.message || 'Failed to save');
        } finally {
            setSaving(false);
        }
    };

    return (
        <Popover
            open={anchorEl !== null}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            slotProps={{ paper: { sx: { p: 1.5, width: 380 } } }}
            // The popover lives inside a clickable model card — keep its
            // gestures to itself.
            onClick={(e) => e.stopPropagation()}
        >
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                Custom Headers
            </Typography>
            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 1 }}>
                Sent only with requests for <Box component="span" sx={{ fontFamily: 'monospace' }}>{model}</Box>.
                Overrides same-name provider-level headers; a rule-level header still wins.
            </Typography>
            <HeadersEditor value={draft} onChange={setDraft} disabled={saving} />
            {error && <Alert severity="error" sx={{ mt: 1, py: 0 }}>{error}</Alert>}
            <Stack direction="row" spacing={1} sx={{ justifyContent: 'flex-end', mt: 1.5 }}>
                <Button size="small" onClick={onClose} disabled={saving}>Cancel</Button>
                <Button size="small" variant="contained" onClick={handleSave} disabled={saving}>
                    {saving ? <CircularProgress size={16} thickness={4} /> : 'Save'}
                </Button>
            </Stack>
        </Popover>
    );
};
