import { cleanup, fireEvent, render, screen, waitForElementToBeRemoved, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import SelectWithCustom from './SelectWithCustom';

afterEach(cleanup);

const custom = {
    label: 'Custom…', dialogTitle: 'Rows', inputLabel: 'Rows input', confirmLabel: 'Apply', cancelLabel: 'Cancel',
    parse: (raw: string) => (/^\d+$/.test(raw) && Number(raw) <= 1000 ? { value: Number(raw) } : { error: 'bad' }),
    toInput: (n: number) => String(n), format: (n: number) => `${n} rows`,
};
const options = [{ value: 200, label: '200 rows' }, { value: 500, label: '500 rows' }];

function Host({ initial = 200, onChange = () => {} }: { initial?: number; onChange?: (n: number) => void }) {
    const [v, setV] = useState(initial);
    return <SelectWithCustom<number> label="Load" value={v} onChange={(n) => { setV(n); onChange(n); }} options={options} custom={custom} />;
}

const openMenu = () => fireEvent.mouseDown(screen.getByRole('combobox'));

describe('SelectWithCustom', () => {
    it('picks a preset without opening the prompt', () => {
        const onChange = vi.fn();
        render(<Host onChange={onChange} />);
        openMenu();
        fireEvent.click(within(screen.getByRole('listbox')).getByText('500 rows'));
        expect(onChange).toHaveBeenCalledWith(500);
        expect(screen.queryByRole('dialog')).toBeNull();
    });

    it('validates, applies a custom value, and shows it as the selected entry', async () => {
        const onChange = vi.fn();
        render(<Host onChange={onChange} />);
        openMenu();
        fireEvent.click(within(screen.getByRole('listbox')).getByText('Custom…'));
        const input = screen.getByRole('textbox', { name: 'Rows input' });
        fireEvent.change(input, { target: { value: 'abc' } });
        expect(screen.getByText('bad')).toBeInTheDocument();
        fireEvent.change(input, { target: { value: '300' } });
        fireEvent.submit(input);
        expect(onChange).toHaveBeenCalledWith(300);
        await waitForElementToBeRemoved(() => screen.queryByRole('dialog'));
        expect(screen.getByRole('combobox')).toHaveTextContent('300 rows');
        openMenu();
        const selected = within(screen.getByRole('listbox')).getByRole('option', { selected: true });
        expect(selected).toHaveTextContent('300 rows');
    });

    it('cancel leaves the value alone, and the prompt starts from the current custom value', async () => {
        const onChange = vi.fn();
        render(<Host initial={300} onChange={onChange} />);
        openMenu();
        fireEvent.click(within(screen.getByRole('listbox')).getByText('Custom…'));
        expect(screen.getByRole('textbox', { name: 'Rows input' })).toHaveValue('300');
        fireEvent.click(screen.getByText('Cancel'));
        await waitForElementToBeRemoved(() => screen.queryByRole('dialog'));
        expect(onChange).not.toHaveBeenCalled();
        expect(screen.getByRole('combobox')).toHaveTextContent('300 rows');
    });
});
