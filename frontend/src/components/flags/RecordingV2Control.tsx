import { Check as IconCheck, KeyboardArrowDown as IconChevronDown, Circle as IconCircleFilled } from '@/components/icons';
import { Box, Button, Checkbox, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';

// Capture points along the gateway pipeline (multi-select). Mirrors the
// backend registry options (typ.RuleFlagRegistry "recording"). Response-side
// points exist in the value domain but are not offered yet — no dead toggles
// (.design/recording.md §3.5):
//   - client_response (final): capture quality not good enough; emit paused.
//     { value: 'client_response', label: 'Client response (final)', short: 'Resp', description: 'The response as returned to the client' },
//   - upstream_response (provider raw): no capture until the wire recorder lands.
export const RECORDING_POINTS = [
    { value: 'client_request', label: 'Client request (inbound)', short: 'In', description: 'The request exactly as the client sent it (before transforms)' },
    { value: 'upstream_request', label: 'Upstream request (outbound)', short: 'Out', description: 'The final request dispatched to the provider (after all transforms)' },
] as const;

// Legacy single-enum values (pre point-set model) mapped onto point sets, so
// configs written before the multi-select model display correctly.
const LEGACY_MODES: Record<string, string[]> = {
    request: ['upstream_request'],
    request_response: ['upstream_request', 'client_response'],
    staged_request_response: ['client_request', 'upstream_request', 'client_response'],
};

// ALL_POINTS is the full backend value domain in canonical (pipeline) order —
// wider than RECORDING_POINTS so stored response-side selections survive
// normalization and toggling even while their checkboxes are not offered.
const ALL_POINTS = ['client_request', 'upstream_request', 'upstream_response', 'client_response'];

// normalizePoints parses a stored recording value (comma-separated points or
// a legacy enum value) into the selected point list, canonical order.
export function normalizePoints(value: string): string[] {
    const seen = new Set<string>();
    for (const tok of (value || '').split(',').map((s) => s.trim()).filter(Boolean)) {
        const legacy = LEGACY_MODES[tok];
        if (legacy) {
            legacy.forEach((p) => seen.add(p));
        } else if (ALL_POINTS.includes(tok)) {
            seen.add(tok);
        }
    }
    return ALL_POINTS.filter((p) => seen.has(p));
}

interface RecordingV2ControlProps {
    value: string;
    disabled?: boolean;
    onChange: (mode: string) => void;
}

const RecordingV2Control: React.FC<RecordingV2ControlProps> = ({ value, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    const selected = normalizePoints(value);
    const isActive = selected.length > 0;
    const shortLabel = isActive
        ? RECORDING_POINTS.filter((p) => selected.includes(p.value)).map((p) => p.short).join('+')
        : 'Off';

    const toggle = (point: string) => {
        const next = selected.includes(point)
            ? selected.filter((p) => p !== point)
            : [...selected, point];
        // Keep canonical order on the wire.
        onChange(ALL_POINTS.filter((p) => next.includes(p)).join(','));
    };

    return (
        <>
            <Tooltip
                title={isActive
                    ? `Recording: ${RECORDING_POINTS.filter((p) => selected.includes(p.value)).map((p) => p.label).join(', ')}`
                    : 'Recording disabled — pick the capture points to record'}
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
                    <IconCircleFilled sx={{ fontSize: 14, mr: '4px' }} />
                    Record: {shortLabel}
                </Button>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {RECORDING_POINTS.map((point) => {
                    const checked = selected.includes(point.value);
                    return (
                        <MenuItem
                            key={point.value}
                            onClick={() => toggle(point.value)}
                            title={point.description}
                            dense
                        >
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                                <Checkbox size="small" checked={checked} sx={{ p: 0 }} />
                                <ListItemText primary={point.label} slotProps={{ primary: { variant: 'body2' } }} />
                            </Box>
                        </MenuItem>
                    );
                })}
                <MenuItem
                    disabled={!isActive}
                    onClick={() => { onChange(''); setAnchor(null); }}
                    dense
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                        <ListItemText primary="Turn off" slotProps={{ primary: { variant: 'body2' } }} />
                        {!isActive && <IconCheck sx={{ fontSize: 16 }} />}
                    </Box>
                </MenuItem>
            </Menu>
        </>
    );
};

export default RecordingV2Control;
