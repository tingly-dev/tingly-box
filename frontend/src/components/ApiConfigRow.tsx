import {
    Box,
    IconButton,
    Stack,
    Tooltip,
    Typography
} from '@mui/material';

interface ApiConfigRowProps {
    label: string;
    value?: string;
    onCopy?: () => void;
    children?: React.ReactNode;
}

const maskValue = (value: string): string => {
    if (value.length <= 16) return value;
    const start = value.slice(0, 12);
    const end = value.slice(-12);
    const res = `${start}${'*'.repeat(8)}${end}`;
    console.log(res)
    return res
};

export const ApiConfigRow: React.FC<ApiConfigRowProps> = ({
    label,
    value,
    onCopy,
    children,
}) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0, maxWidth: 700 }}>
        <Typography
            variant="subtitle2"
            color="text.secondary"
            sx={{
                minWidth: 190,
                flexShrink: 0,
                fontWeight: 500
            }}
        >
            {label}:
        </Typography>
        <Typography
            variant="subtitle2"
            onClick={onCopy ? onCopy : undefined}
            sx={{
                fontFamily: 'monospace',
                fontSize: '0.75rem',
                color: 'primary.main',
                flex: 1,
                minWidth: 0,
                cursor: 'pointer',
                '&:hover': {
                    textDecoration: 'underline',
                    backgroundColor: 'action.hover'
                },
                padding: 1,
                borderRadius: 1,
                transition: 'all 0.2s ease-in-out'
            }}
            title={`Click to copy ${label}`}
        >
            {value ? maskValue(value) : value}
        </Typography>
        <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0, ml: 'auto' }}>
            {children}
        </Stack>
    </Box>
);