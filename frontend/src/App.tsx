import { Error as ErrorIcon, Refresh } from '@/components/icons';
import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, IconButton, Paper, Stack, Typography } from '@mui/material';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { Suspense, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { resolveLanguage } from '@/i18n';
import { BrowserRouter, Routes } from 'react-router-dom';
import { AuthProvider } from './contexts/AuthContext';
import { FeatureFlagsProvider } from './contexts/FeatureFlagsContext';
import { HealthProvider, useHealth } from './contexts/HealthContext';
import { NotificationProvider } from './contexts/NotificationContext';
import { ThemeModeProvider, useThemeMode } from './contexts/ThemeContext';
import { useVersion, VersionProvider } from './contexts/VersionContext';
import { ProfileProvider } from './contexts/ProfileContext';
import { TeamProvider } from './contexts/TeamContext';
import { appRoutes } from './routes/appRoutes';
import createAppTheme from './theme';
import { elevation } from '@/theme/elevation';


// Route-switch fallback: Layout/nav chrome is already on screen (it renders
// outside this Suspense boundary), so this only covers the content area
// while a page chunk downloads — a brief spinner, not a full-page blank.
const RouteFallback = () => (
    <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '40vh' }}>
        <CircularProgress />
    </Box>
);

// Dialogs component that uses the health context
const AppDialogs = () => {
    const { t } = useTranslation();
    const { isHealthy, checking, checkHealth, disconnectDialogOpen, closeDisconnectDialog } = useHealth();

    return (
        <>
            {/* Disconnect Alert Dialog - now manually controlled */}
            <Dialog
                open={disconnectDialogOpen}
                onClose={closeDisconnectDialog}
                maxWidth="sm"
                fullWidth
                slotProps={{
                    paper: {
                        sx: {
                            borderRadius: 2,
                            boxShadow: elevation.overlay,
                        }
                    }
                }}
            >
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <ErrorIcon color="error" />
                    {t('health.disconnectTitle', { defaultValue: 'Connection Lost' })}
                </DialogTitle>
                <DialogContent>
                    <Typography variant="body1">
                        {t('health.disconnectMessage', { defaultValue: 'Connection to server lost. Please check if the server is running.' })}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeDisconnectDialog}>
                        {t('common.close', { defaultValue: 'Close' })}
                    </Button>
                    <Button
                        variant="contained"
                        onClick={checkHealth}
                        disabled={checking}
                        startIcon={checking ? <CircularProgress size={16} /> : <Refresh />}
                    >
                        {t('health.retry', { defaultValue: 'Retry' })}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

function AppContent() {
    return (
        <Suspense fallback={<RouteFallback />}>
            <Routes>
                {appRoutes}
            </Routes>
        </Suspense>
    )
}

// Inner component that uses theme context
function AppWithTheme() {
    const { effectiveMode } = useThemeMode();
    const { i18n } = useTranslation();
    const language = resolveLanguage(i18n.language);
    const theme = useMemo(() => createAppTheme(effectiveMode, language), [effectiveMode, language]);

    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <NotificationProvider>
                <BrowserRouter>
                    <HealthProvider>
                        <VersionProvider>
                            <AuthProvider>
                                <FeatureFlagsProvider>
                                    <ProfileProvider>
                                        <TeamProvider>
                                            <AppContent />
                                            <AppDialogs />
                                        </TeamProvider>
                                    </ProfileProvider>
                                </FeatureFlagsProvider>
                            </AuthProvider>
                        </VersionProvider>
                    </HealthProvider>
                </BrowserRouter>
            </NotificationProvider>
        </ThemeProvider>
    );
}

function App() {
    return (
        <ThemeModeProvider>
            <AppWithTheme />
        </ThemeModeProvider>
    );
}

export default App;
