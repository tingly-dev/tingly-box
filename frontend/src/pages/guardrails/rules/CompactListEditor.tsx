import {
    Box,
    FormHelperText,
    IconButton,
    InputBase,
    Paper,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Typography,
} from '@mui/material';
import { Add, Remove } from '@/components/icons';
import { appendTextListValue, removeTextListValue, textListRows, updateTextListValue } from './listField';
import type { OversizedListField } from './types';

type CompactListEditorProps = {
    title: string;
    description: string;
    columnLabel: string;
    value: string;
    selectedIndex: number;
    onSelectedIndexChange: (index: number) => void;
    onChange: (value: string) => void;
    placeholder: string;
    helperText: string;
    oversizedField?: OversizedListField;
};

const CompactListEditor = ({
    title,
    description,
    columnLabel,
    value,
    selectedIndex,
    onSelectedIndexChange,
    onChange,
    placeholder,
    helperText,
    oversizedField,
}: CompactListEditorProps) => {
    const rows = textListRows(value);
    const isEmpty = rows.length === 1 && rows[0] === '';
    const showEmptyState = isEmpty && selectedIndex < 0;
    const visibleRows = showEmptyState ? [] : rows;
    const editablePrefixCount = oversizedField ? Math.max(0, rows.length - oversizedField.preview.length) : rows.length;
    const canRemove = !showEmptyState && (!oversizedField || selectedIndex < editablePrefixCount);

    return (
        <Stack spacing={1.5}>
            <Box>
                <Typography variant="subtitle2">{title}</Typography>
                <Typography variant="caption" sx={{
                    color: "text.secondary"
                }}>
                    {description}
                </Typography>
            </Box>
            <TableContainer component={Paper} variant="outlined" sx={{ borderRadius: 2, boxShadow: 'none' }}>
                <Stack
                    direction="row"
                    spacing={0.5}
                    sx={{
                        justifyContent: "space-between",
                        alignItems: "center",
                        px: 1,
                        py: 0.5,
                        borderBottom: '1px solid',
                        borderColor: 'divider',
                        bgcolor: 'action.hover'
                    }}>
                    <Stack direction="row" spacing={0.5}>
                        <IconButton
                            size="small"
                            color="primary"
                            onClick={() => {
                                if (showEmptyState) {
                                    onSelectedIndexChange(0);
                                    return;
                                }
                                if (oversizedField) {
                                    onChange(['', ...rows].join('\n'));
                                    onSelectedIndexChange(0);
                                    return;
                                }
                                onChange(appendTextListValue(value));
                                onSelectedIndexChange(rows.length);
                            }}
                        >
                            <Add fontSize="small" />
                        </IconButton>
                        <IconButton
                            size="small"
                            disabled={!canRemove}
                            onClick={() => {
                                if (showEmptyState) {
                                    return;
                                }
                                if (oversizedField && selectedIndex >= editablePrefixCount) {
                                    return;
                                }
                                const index = Math.min(selectedIndex, rows.length - 1);
                                const nextValue = removeTextListValue(value, index);
                                onChange(nextValue);
                                const nextRows = textListRows(nextValue);
                                if (nextRows.length === 1 && nextRows[0] === '') {
                                    onSelectedIndexChange(-1);
                                } else {
                                    onSelectedIndexChange(Math.max(0, Math.min(selectedIndex - 1, nextRows.length - 1)));
                                }
                            }}
                        >
                            <Remove fontSize="small" />
                        </IconButton>
                    </Stack>
                </Stack>
                <Table size="small">
                    <TableHead>
                        <TableRow>
                            <TableCell sx={{ fontWeight: 600 }}>{columnLabel}</TableCell>
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {showEmptyState ? (
                            <TableRow>
                                <TableCell sx={{ py: 3, textAlign: 'center', color: 'text.secondary' }}>
                                    No entries
                                </TableCell>
                            </TableRow>
                        ) : (
                            visibleRows.map((item, index) => (
                                <TableRow
                                    key={`${title}-${index}`}
                                    hover
                                    selected={selectedIndex === index}
                                    onClick={() => onSelectedIndexChange(index)}
                                    sx={{ cursor: 'pointer' }}
                                >
                                    <TableCell sx={{ py: 0.5 }}>
                                        <InputBase
                                            fullWidth
                                            value={item}
                                            readOnly={Boolean(oversizedField) && index >= editablePrefixCount}
                                            placeholder={index === 0 ? placeholder : 'Add another entry'}
                                            onFocus={() => onSelectedIndexChange(index)}
                                            onChange={(e) => onChange(updateTextListValue(value, index, e.target.value))}
                                            sx={{ fontSize: '0.9rem' }}
                                        />
                                    </TableCell>
                                </TableRow>
                            ))
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
            <FormHelperText>
                {oversizedField
                    ? `This field contains ${oversizedField.total.toLocaleString()} entries. Showing the first ${oversizedField.preview.length} existing entries. Existing rows stay read-only, but you can prepend new entries without loading the full list. Saving preserves the rest of the list.`
                    : helperText}
            </FormHelperText>
        </Stack>
    );
};

export default CompactListEditor;
