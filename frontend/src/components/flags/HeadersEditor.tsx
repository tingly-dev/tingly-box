import { Add as AddIcon, Delete as DeleteIcon } from '@/components/icons';
import { Box, Button, IconButton, Stack, TextField, Typography } from '@mui/material';
import React, { useMemo, useState } from 'react';
import { fontMono, fontSizes } from '@/theme/fonts';

// RFC 7230 header-name token characters.
const TOKEN_RE = /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/;

// Structural validation only — headers are user-driven config, so there is no
// denylist: any header can be set (including gateway-adjacent ones like
// Authorization or User-Agent) and the user owns the outcome. Mirrors
// typ.ValidateExtraHeaders.
export function headerNameError(name: string, seenLower: Set<string>): string | undefined {
    if (name === '') return undefined; // incomplete row, not an error yet
    if (!TOKEN_RE.test(name)) return 'Invalid characters for an HTTP header name';
    if (seenLower.has(name.toLowerCase())) return 'Duplicate header name (names are case-insensitive)';
    return undefined;
}

interface HeaderRow {
    id: number;
    name: string;
    value: string;
}

export interface HeadersEditorProps {
    /** Current header map (empty/undefined = none configured). */
    value?: Record<string, string>;
    /** Called with the full replacement map on every edit (invalid/incomplete rows excluded). */
    onChange: (next: Record<string, string> | undefined) => void;
    disabled?: boolean;
}

let nextRowId = 1;

const rowsFromValue = (value?: Record<string, string>): HeaderRow[] =>
    Object.entries(value ?? {}).map(([name, v]) => ({ id: nextRowId++, name, value: v }));

/**
 * HeadersEditor — the shared key/value row editor for headers-type flags,
 * used by the rule catalog, the provider Plugins section, and the per-model
 * override popover. Rows validate structure inline (illegal characters /
 * duplicates — never a denylist); rows with an empty or invalid name are
 * kept visible for editing but excluded from the emitted map.
 */
export const HeadersEditor: React.FC<HeadersEditorProps> = ({ value, onChange, disabled }) => {
    // Seeded once per mount; dialogs/popovers remount per open, so the parent
    // draft is the single source of truth after the first edit.
    const [rows, setRows] = useState<HeaderRow[]>(() => rowsFromValue(value));

    const emit = (next: HeaderRow[]) => {
        setRows(next);
        const map: Record<string, string> = {};
        const seen = new Set<string>();
        for (const row of next) {
            const name = row.name.trim();
            if (name === '' || headerNameError(name, new Set()) !== undefined) continue;
            const lower = name.toLowerCase();
            if (seen.has(lower)) continue;
            seen.add(lower);
            map[name] = row.value;
        }
        onChange(Object.keys(map).length > 0 ? map : undefined);
    };

    const errors = useMemo(() => {
        const seen = new Set<string>();
        return rows.map((row) => {
            const err = headerNameError(row.name.trim(), seen);
            if (row.name.trim() !== '') seen.add(row.name.trim().toLowerCase());
            return err;
        });
    }, [rows]);

    return (
        <Stack spacing={0.75}>
            {rows.map((row, i) => (
                <Stack key={row.id} direction="row" spacing={0.75} sx={{ alignItems: 'flex-start' }}>
                    <TextField
                        size="small"
                        placeholder="Header-Name"
                        value={row.name}
                        disabled={disabled}
                        error={!!errors[i]}
                        helperText={errors[i]}
                        onChange={(e) => emit(rows.map((r) => r.id === row.id ? { ...r, name: e.target.value } : r))}
                        sx={{ flex: '0 0 40%', '& input': { fontFamily: fontMono, fontSize: fontSizes.md } }}
                    />
                    <TextField
                        size="small"
                        placeholder="value"
                        value={row.value}
                        disabled={disabled}
                        onChange={(e) => emit(rows.map((r) => r.id === row.id ? { ...r, value: e.target.value } : r))}
                        sx={{ flexGrow: 1, '& input': { fontFamily: fontMono, fontSize: fontSizes.md } }}
                    />
                    <IconButton
                        size="small"
                        disabled={disabled}
                        onClick={() => emit(rows.filter((r) => r.id !== row.id))}
                        sx={{ mt: 0.25, color: 'text.disabled', '&:hover': { color: 'error.main' } }}
                    >
                        <DeleteIcon fontSize="small" />
                    </IconButton>
                </Stack>
            ))}
            {rows.length === 0 && (
                <Typography variant="caption" sx={{ color: 'text.disabled' }}>
                    No custom headers.
                </Typography>
            )}
            <Box>
                <Button
                    size="small"
                    startIcon={<AddIcon fontSize="small" />}
                    disabled={disabled}
                    onClick={() => emit([...rows, { id: nextRowId++, name: '', value: '' }])}
                    sx={{ textTransform: 'none' }}
                >
                    Add header
                </Button>
            </Box>
        </Stack>
    );
};

export default HeadersEditor;
