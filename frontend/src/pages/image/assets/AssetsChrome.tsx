import { InputAdornment, Paper, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Search } from '@/components/icons';

// The bits both Assets tabs are built from, so the two read as one page.

export const SearchField: React.FC<{ value: string; onChange: (value: string) => void; placeholder: string }> = ({
    value,
    onChange,
    placeholder,
}) => (
    <TextField
        size="small"
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        sx={{ flex: 1, maxWidth: { sm: 360 } }}
        slotProps={{
            input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> },
        }}
    />
);

// What an empty tab says: what goes here, and the ways it gets here.
export const EmptyState: React.FC<{ title: string; body: string }> = ({ title, body }) => (
    <Paper variant="outlined" sx={{ p: 3, textAlign: 'center', borderStyle: 'dashed' }}>
        <Typography variant="subtitle1" sx={{ mb: 1 }}>{title}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 560, mx: 'auto' }}>{body}</Typography>
    </Paper>
);

export const NoMatches: React.FC = () => {
    const { t } = useTranslation();
    return (
        <Typography variant="body2" color="text.secondary" sx={{ py: 3, textAlign: 'center' }}>
            {t('imageAssets.noMatches', { defaultValue: 'Nothing matches' })}
        </Typography>
    );
};
