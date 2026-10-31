import { useState } from 'react';
import { Box, Button, Divider, ListItemText, Menu, MenuItem, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Model } from '../client';
import { Check, ExpandMore } from './icons';
import { useModels, type ModelSource } from '../hooks/useModels';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

interface Props {
    value: string;
    onChange: (model: string) => void;
    disabled?: boolean;
    /** What the model is for, shown before it when a box holds more than one picker. */
    label?: string;
    source?: ModelSource;
    /** Empty is inheritance, distinct from explicitly choosing the same model. */
    defaultLabel?: string;
    /** Project default used to resolve the inherited alias when available. */
    inheritedModel?: string;
}

/** The real model a listed id stands for, when the host routes it to one
 * (provider_model is empty when routes are unresolved or the id is its own). */
const routedTo = (m: Model | undefined) => (m && m.provider_model && m.provider_model !== m.id ? m.provider_model : undefined);

/**
 * Shows the concrete model ID that will be called (not an alias) and lets the
 * user switch. When the endpoint cannot list models, a free-text field takes
 * over so chatting is never blocked on discovery. A listed id the host routes
 * (an alias) is shown with the real model it stands for, the way the status
 * line renders a route.
 */
export function ModelPicker({ value, onChange, disabled, label, defaultLabel, inheritedModel, source = 'connection' }: Props) {
    const { t } = useTranslation('projects');
    const { models, error } = useModels(source);
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [custom, setCustom] = useState('');

    // What the picked id routes to: the alias "sonnet" shows the real model
    // beside it, both on the button and in the menu.
    const real = routedTo(models.find((m) => m.id === value));
    const shown = real ? `${value} → ${real}` : value;
    const inheritedReal = routedTo(models.find((m) => m.id === inheritedModel));
    const inheritedLabel = inheritedReal && defaultLabel ? `${defaultLabel} → ${inheritedReal}` : defaultLabel;

    const close = () => setAnchor(null);
    return (
        <>
            <Button
                size="small"
                color="inherit"
                disabled={disabled}
                onClick={(e) => setAnchor(e.currentTarget)}
                endIcon={<ExpandMore fontSize="small" />}
                sx={{ color: 'text.secondary', fontWeight: 500, fontSize: fontSizes.sm, maxWidth: 260, px: 0.75, minWidth: 0, borderRadius: radius.row }}
            >
                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={shown}>
                    {label ? `${label} · ` : ''}{shown || inheritedLabel || t('model.auto')}
                </Box>
            </Button>
            <Menu
                anchorEl={anchor}
                open={!!anchor && !disabled}
                onClose={close}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                slotProps={{ paper: { sx: { maxHeight: 420, minWidth: 280 } } }}
            >
                {(defaultLabel !== undefined || source === 'projects') && <MenuItem selected={value === ''} onClick={() => { if (!disabled) onChange(''); close(); }}>
                    <ListItemText primary={inheritedLabel || t('model.auto')} />
                    {value === '' && <Check fontSize="small" color="primary" sx={{ ml: 1 }} />}
                </MenuItem>}
                {models.map((m) => {
                    const routed = routedTo(m);
                    const real = routed && [routed, m.provider_name].filter(Boolean).join(' @ ');
                    return (
                        <MenuItem key={m.id} selected={m.id === value} onClick={() => { if (!disabled) { onChange(m.id); close(); } }}>
                            <ListItemText
                                primary={m.id}
                                secondary={real ?? (m.display_name && m.display_name !== m.id ? m.display_name : undefined)}
                                slotProps={{ primary: { sx: { fontFamily: fontMono, fontSize: fontSizes.base } }, secondary: { sx: { fontFamily: real ? fontMono : undefined, fontSize: fontSizes.sm } } }}
                            />
                            {m.id === value && <Check fontSize="small" color="primary" sx={{ ml: 1 }} />}
                        </MenuItem>
                    );
                })}
                {error && (
                    <Box sx={{ px: 2, py: 1, maxWidth: 320 }}>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
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
                        if (!disabled && custom.trim()) {
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
                        slotProps={{ htmlInput: { style: { fontFamily: fontMono, fontSize: fontSizes.base } } }}
                    />
                </Box>
            </Menu>
        </>
    );
}
