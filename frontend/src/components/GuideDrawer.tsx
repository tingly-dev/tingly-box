import { Close } from '@/components/icons';
import {
    Box,
    Button,
    Divider,
    Drawer,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface GuideDrawerProps {
    open: boolean;
    title: string;
    description?: string;
    children: ReactNode;
    onClose: () => void;
    actionLabel?: string;
    onAction?: () => void;
    width?: number;
}

const GuideDrawer = ({
    open,
    title,
    description,
    children,
    onClose,
    actionLabel,
    onAction,
    width = 440,
}: GuideDrawerProps) => {
    const { t } = useTranslation();

    const handleAction = () => {
        onClose();
        onAction?.();
    };

    return (
        <Drawer
            anchor="right"
            open={open}
            onClose={onClose}
            // A guide can be opened from inside a dialog; keep it the top
            // layer instead of sliding in underneath the dialog's backdrop.
            sx={(theme) => ({ zIndex: theme.zIndex.modal + 1 })}
            slotProps={{
                paper: {
                    sx: {
                        width: { xs: '100%', sm: width },
                        maxWidth: '100%',
                    },
                },
            }}
        >
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    height: '100%',
                }}
            >
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'flex-start',
                        justifyContent: 'space-between',
                        gap: 2,
                        px: 3,
                        py: 2.5,
                    }}
                >
                    <Box sx={{ minWidth: 0 }}>
                        <Typography variant="h4" component="h2" sx={{ fontWeight: 600 }}>
                            {title}
                        </Typography>
                        {description && (
                            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                                {description}
                            </Typography>
                        )}
                    </Box>
                    <IconButton
                        aria-label={t('common.close', { defaultValue: 'Close' })}
                        onClick={onClose}
                        edge="end"
                    >
                        <Close />
                    </IconButton>
                </Box>
                <Divider />
                <Box sx={{ flex: 1, overflowY: 'auto', px: 3, py: 2.5 }}>
                    <Stack spacing={2}>{children}</Stack>
                </Box>
                {actionLabel && onAction && (
                    <>
                        <Divider />
                        <Box sx={{ px: 3, py: 2 }}>
                            <Button fullWidth variant="contained" onClick={handleAction}>
                                {actionLabel}
                            </Button>
                        </Box>
                    </>
                )}
            </Box>
        </Drawer>
    );
};

export default GuideDrawer;
