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
    isClickable?: boolean;
    isMonospace?: boolean;
    showEllipsis?: boolean;
}

export const ApiConfigRow: React.FC<ApiConfigRowProps> = ({
    label,
    value,
    onCopy,
    children,
    isClickable = false,
    isMonospace = true,
    showEllipsis = false
}) => (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, maxWidth: 500 }}>
        <Typography
            variant="body2"
            color="text.secondary"
            sx={{
                minWidth: 120,
                flexShrink: 0,
                fontWeight: 500
            }}
        >
            {label}:
        </Typography>
        <Typography
            variant="body2"
            onClick={isClickable && onCopy ? onCopy : undefined}
            sx={{
                fontFamily: isMonospace ? 'monospace' : 'inherit',
                fontSize: showEllipsis ? '0.8rem' : '0.75rem',
                color: isClickable ? 'primary.main' : 'text.secondary',
                letterSpacing: showEllipsis ? '2px' : 'normal',
                flex: 'none',
                maxWidth: '300px',
                cursor: isClickable ? 'pointer' : 'default',
                userSelect: showEllipsis ? 'none' : 'auto',
                '&:hover': isClickable ? {
                    textDecoration: 'underline',
                    backgroundColor: 'action.hover'
                } : {},
                padding: isClickable ? 1 : 0,
                borderRadius: isClickable ? 1 : 0,
                transition: 'all 0.2s ease-in-out'
            }}
            title={isClickable ? `Click to copy ${label}` : undefined}
        >
            {showEllipsis ? '••••••••••••••••' : value}
        </Typography>
        <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0, minWidth: 'fit-content', marginLeft: 'auto' }}>
            {children}
        </Stack>
    </Box>
);