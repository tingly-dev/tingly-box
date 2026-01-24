import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { CircularProgress, Box } from '@mui/material';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import Layout from './layout/Layout';
import theme from './theme';

// Lazy load pages for code splitting
const Login = lazy(() => import('./pages/Login'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const UseOpenAIPage = lazy(() => import('./pages/UseOpenAIPage'));
const UseAnthropicPage = lazy(() => import('./pages/UseAnthropicPage'));
const UseClaudeCodePage = lazy(() => import('./pages/UseClaudeCodePage'));
const ApiKeyPage = lazy(() => import('./pages/ApiKeyPage'));
const OAuthPage = lazy(() => import('./pages/OAuthPage'));
const System = lazy(() => import('./pages/System'));
const UsageDashboardPage = lazy(() => import('./pages/UsageDashboardPage'));
const ModelTestPage = lazy(() => import('./pages/ModelTestPage'));

// Loading fallback component
const PageLoader = () => (
    <Box
        sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            height: '100vh',
        }}
    >
        <CircularProgress />
    </Box>
);

function App() {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter>
                <AuthProvider>
                    <Suspense fallback={<PageLoader />}>
                        <Routes>
                            <Route path="/login" element={<Login />} />
                            <Route
                                path="/*"
                                element={
                                    <ProtectedRoute>
                                        <Layout>
                                            <Suspense fallback={<PageLoader />}>
                                                <Routes>
                                                    <Route path="/" element={<Dashboard />} />
                                                    {/* Function panel routes */}
                                                    <Route path="/use-openai" element={<UseOpenAIPage />} />
                                                    <Route path="/use-anthropic" element={<UseAnthropicPage />} />
                                                    <Route path="/use-claude-code" element={<UseClaudeCodePage />} />
                                                    {/* Other routes */}
                                                    <Route path="/api-keys" element={<ApiKeyPage />} />
                                                    <Route path="/oauth" element={<OAuthPage />} />
                                                    <Route path="/system" element={<System />} />
                                                    <Route path="/dashboard" element={<UsageDashboardPage />} />
                                                    <Route path="/model-test/:providerUuid" element={<ModelTestPage />} />
                                                </Routes>
                                            </Suspense>
                                        </Layout>
                                    </ProtectedRoute>
                                }
                            />
                        </Routes>
                    </Suspense>
                </AuthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
