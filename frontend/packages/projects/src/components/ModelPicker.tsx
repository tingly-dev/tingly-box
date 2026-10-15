import { useState } from 'react';
import { Box, Button, Divider, ListItemText, Menu, MenuItem, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Check, ExpandMore } from './icons';
import { useModels } from '../hooks/useModels';

interface Props {
    value: string;
    onChange: (model: string) => void;
    disabled?: boolean;
}

/**
 * Shows the concrete model ID that will be called (not an alias) and lets the
 * user switch. When the endpoint cannot list models, a free-text field takes
 * over so chatting is never blocked on discovery.
 */
export function ModelPicker({ value, onChange, disabled }: Props) {
    const { t } = useTranslation();
    const { models, error } = useModels();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [custom, setCustom] = useState('');

    const close = () => setAnchor(null);
    return (
        <>
            <Button
                size="small"
                color="inherit"
                disabled={disabled}
                onClick={(e) => setAnchor(e.currentTarget)}
                endIcon={<ExpandMore fontSize="small" />}
                sx={{ color: 'text.secondary', fontWeight: 500, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 12.5, maxWidth: 260 }}
            >
                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {value || t('model.auto')}
                </Box>
            </Button>
            <Menu
                anchorEl={anchor}
                open={!!anchor}
                onClose={close}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                slotProps={{ paper: { sx: { maxHeight: 420, minWidth: 280 } } }}
            >
                {models.map((m) => (
                    <MenuItem key={m.id} selected={m.id === value} onClick={() => { onChange(m.id); close(); }}>
                        <ListItemText
                            primary={m.id}
                            secondary={m.display_name && m.display_name !== m.id ? m.display_name : undefined}
                            slotProps={{ primary: { sx: { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 13 } } }}
                        />
                        {m.id === value && <Check fontSize="small" color="primary" sx={{ ml: 1 }} />}
                    </MenuItem>
                ))}
                {error && (
                    <Box sx={{ px: 2, py: 1, maxWidth: 320 }}>
                        <Typography variant="caption" color="warning.main" sx={{ display: 'block' }}>
                            {t('model.listFailed')}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-word' }}>
                            {error}
                        </Typography>
                    </Box>
                )}
                {models.length > 0 && <Divider />}
                <Box
                    component="form"
                    sx={{ px: 2, py: 1 }}
                    onSubmit={(e) => {
                        e.preventDefault();
                        if (custom.trim()) {
                            onChange(custom.trim());
                            setCustom('');
                            close();
                        }
                    }}
                    onKeyDown={(e) => e.stopPropagation()}
                >
                    <TextField
                        size="small"
                        fullWidth
                        placeholder={t('model.customPlaceholder')}
                        value={custom}
                        onChange={(e) => setCustom(e.target.value)}
                        slotProps={{ htmlInput: { style: { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace', fontSize: 13 } } }}
                    />
                </Box>
            </Menu>
        </>
    );
}
