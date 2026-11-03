import { useState } from 'react';
import { Box, Button, ListItemText, Menu, MenuItem } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Check, ExpandMore } from './icons';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

// The effort levels a project can set, the ladder the gateway's rules speak;
// empty is the endpoint's default.
const EFFORTS = ['low', 'medium', 'high', 'max'] as const;

// The menu's choices: value and its i18n key, default first.
const CHOICES: ReadonlyArray<readonly [string, string]> = [
    ['', 'effort.default'],
    ...EFFORTS.map((level) => [level, `effort.${level}`] as const),
];

interface Props {
    value: string;
    onChange: (effort: string) => void;
    disabled?: boolean;
    /** What the effort is for, shown before it when a box holds more than one picker. */
    label?: string;
    /** Text for the inherit option and its compact value in the composer. */
    defaultLabel?: string;
    defaultDisplayLabel?: string;
}

/**
 * Picks the thinking effort beside a model picker (the same quiet button
 * style). Empty is the endpoint's default, distinct from choosing a level.
 */
export function EffortPicker({ value, onChange, disabled, label, defaultLabel, defaultDisplayLabel }: Props) {
    const { t } = useTranslation('projects');
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    const close = () => setAnchor(null);
    return (
        <>
            <Button
                size="small"
                color="inherit"
                disabled={disabled}
                onClick={(e) => setAnchor(e.currentTarget)}
                endIcon={<ExpandMore fontSize="small" />}
                sx={{ color: 'text.secondary', fontWeight: 500, fontSize: fontSizes.sm, px: 0.75, minWidth: 0, borderRadius: radius.row }}
            >
                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {label ? `${label} · ` : ''}{value ? t(`effort.${value}`) : (defaultDisplayLabel || defaultLabel || t('effort.default'))}
                </Box>
            </Button>
            <Menu
                anchorEl={anchor}
                open={!!anchor && !disabled}
                onClose={close}
                anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
                transformOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                slotProps={{ paper: { sx: { minWidth: 240 } } }}
            >
                {CHOICES.map(([level, key]) => (
                    <MenuItem key={key} selected={level === value} onClick={() => { if (!disabled) { onChange(level); close(); } }}>
                        <ListItemText
                            primary={level === '' && defaultLabel ? defaultLabel : t(key)}
                            secondary={t(`${key}Hint`)}
                            slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { fontSize: fontSizes.sm } } }}
                        />
                        {level === value && <Check fontSize="small" color="primary" sx={{ ml: 1 }} />}
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
}
