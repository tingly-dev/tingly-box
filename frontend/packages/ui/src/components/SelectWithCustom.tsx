import {
    Divider, FormControl, InputLabel, ListItemText, MenuItem, Select, type SelectProps,
} from '@mui/material';
import { type ReactNode, useState } from 'react';
import { fontSizes } from '../theme/fonts';
import CustomValueDialog from './CustomValueDialog';

export interface SelectOption<V extends string | number> {
    value: V;
    label: ReactNode;
    /** A second line explaining the choice. */
    secondary?: ReactNode;
}

export interface CustomSpec<V extends string | number> {
    /** The menu entry that opens the prompt ("Custom…"). */
    label: ReactNode;
    dialogTitle: ReactNode;
    inputLabel?: string;
    helperText?: ReactNode;
    unit?: string;
    inputMode?: 'numeric' | 'decimal' | 'text';
    confirmLabel?: ReactNode;
    cancelLabel?: ReactNode;
    /** The typed text as a value, or the error to show. May normalise (e.g. return a preset's value). */
    parse: (raw: string) => { value: V } | { error: string };
    /** The text the prompt starts with for the current value. */
    toInput: (value: V) => string;
    /** How the current value reads in the menu and field when it is not a preset. */
    format: (value: V) => ReactNode;
}

export interface SelectWithCustomProps<V extends string | number>
    extends Omit<SelectProps<V | ''>, 'value' | 'onChange' | 'children' | 'renderValue' | 'label'> {
    value: V;
    onChange: (value: V) => void;
    options: ReadonlyArray<SelectOption<V>>;
    custom: CustomSpec<V>;
    /** A floating label (form field). Leave out for a bare picker. */
    label?: string;
    /** Shown first in the menu, not selectable (a ListSubheader). */
    menuHeader?: ReactNode;
    /** Overrides how the closed field reads; defaults to the option's label or `custom.format`. */
    renderValue?: (value: V) => ReactNode;
}

const CUSTOM = '__custom__';

/**
 * A Select whose list ends in "Custom…": picking it opens a small prompt for
 * a value the presets do not cover. A value outside the presets (a saved
 * custom one) shows as the selected entry, so the field never looks empty and
 * the user can see what they set. Cancelling the prompt changes nothing.
 * The value stored is the host's own type; nothing about "custom" is saved.
 */
export default function SelectWithCustom<V extends string | number>({
    value, onChange, options, custom, label, menuHeader, renderValue, size = 'small', ...rest
}: SelectWithCustomProps<V>) {
    const [open, setOpen] = useState(false);
    const isPreset = options.some((o) => o.value === value);
    const text = { primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { whiteSpace: 'normal' as const, fontSize: fontSizes.sm } } };

    const items = [
        menuHeader,
        ...options.map((o) => (
            <MenuItem key={String(o.value)} value={o.value} sx={{ alignItems: 'flex-start' }}>
                {o.secondary
                    ? <ListItemText primary={o.label} secondary={o.secondary} slotProps={text} />
                    : <ListItemText primary={o.label} slotProps={{ primary: text.primary }} />}
            </MenuItem>
        )),
        isPreset ? null : (
            <MenuItem key="current-custom" value={value}>
                <ListItemText primary={custom.format(value)} slotProps={{ primary: text.primary }} />
            </MenuItem>
        ),
        <Divider key="custom-divider" component="li" />,
        <MenuItem key={CUSTOM} value={CUSTOM}>
            <ListItemText primary={custom.label} slotProps={{ primary: text.primary }} />
        </MenuItem>,
    ];

    const select = (
        <Select<V | ''>
            {...rest}
            size={size}
            label={label}
            value={value}
            onChange={(e) => {
                if (e.target.value === CUSTOM) { setOpen(true); return; }
                onChange(e.target.value as V);
            }}
            renderValue={(v) => {
                const current = v as V;
                if (renderValue) return renderValue(current);
                const preset = options.find((o) => o.value === current);
                return preset ? preset.label : custom.format(current);
            }}
        >
            {items}
        </Select>
    );

    return (
        <>
            {label
                ? (
                    <FormControl size={size} fullWidth={rest.fullWidth} disabled={rest.disabled}>
                        <InputLabel>{label}</InputLabel>
                        {select}
                    </FormControl>
                )
                : select}
            <CustomValueDialog
                open={open}
                title={custom.dialogTitle}
                initial={isPreset ? '' : custom.toInput(value)}
                inputLabel={custom.inputLabel}
                helperText={custom.helperText}
                unit={custom.unit}
                inputMode={custom.inputMode}
                confirmLabel={custom.confirmLabel}
                cancelLabel={custom.cancelLabel}
                validate={(raw) => { const r = custom.parse(raw); return 'error' in r ? r.error : null; }}
                onSubmit={(raw) => {
                    const r = custom.parse(raw);
                    if ('value' in r) { setOpen(false); onChange(r.value); }
                }}
                onCancel={() => setOpen(false)}
            />
        </>
    );
}
