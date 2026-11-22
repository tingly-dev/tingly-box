import SelectWithCustom from '@tingly/ui/components/SelectWithCustom';
import { useTranslation } from 'react-i18next';

const PRESETS = [200, 500, 1000];
// The log APIs cap `limit` at 1000.
const MAX_LIMIT = 1000;

/** "Latest N" row-count picker of the log viewers; Custom… takes any 1 to 1000. */
export default function LogLimitSelect({ value, onChange }: { value: number; onChange: (n: number) => void }) {
    const { t } = useTranslation();
    const latest = (count: number) => t('logViewer.latest', { count, defaultValue: 'Latest {{count}}' });
    return (
        <SelectWithCustom<number>
            value={value}
            onChange={onChange}
            options={PRESETS.map((n) => ({ value: n, label: latest(n) }))}
            inputProps={{ 'aria-label': t('logViewer.load', { defaultValue: 'Load' }) }}
            custom={{
                label: t('logViewer.customLimit', { defaultValue: 'Custom…' }),
                dialogTitle: t('logViewer.customLimitTitle', { defaultValue: 'Number of rows to load' }),
                inputLabel: t('logViewer.customLimitInput', { defaultValue: 'Rows' }),
                helperText: t('logViewer.customLimitHelp', { max: MAX_LIMIT, defaultValue: 'Whole number, 1 to {{max}}.' }),
                inputMode: 'numeric',
                confirmLabel: t('logViewer.customLimitApply', { defaultValue: 'Apply' }),
                cancelLabel: t('logViewer.customLimitCancel', { defaultValue: 'Cancel' }),
                parse: (raw) => {
                    const n = /^\d+$/.test(raw) ? Number(raw) : NaN;
                    return n >= 1 && n <= MAX_LIMIT
                        ? { value: n }
                        : { error: t('logViewer.customLimitInvalid', { max: MAX_LIMIT, defaultValue: 'Enter a whole number from 1 to {{max}}.' }) };
                },
                toInput: (n) => String(n),
                format: latest,
            }}
        />
    );
}
