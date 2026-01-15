import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import Layout from './layout/Layout';
import Dashboard from './pages/Dashboard';
import Home from './pages/Home';
import Login from './pages/Login';
import ApiKeyPage from './pages/ApiKeyPage';
import OAuthPage from './pages/OAuthPage';
import System from './pages/System';
import UseOpenAIPage from './pages/UseOpenAIPage';
import UseAnthropicPage from './pages/UseAnthropicPage';
import UseClaudeCodePage from './pages/UseClaudeCodePage';
import ModelTestPage from './pages/ModelTestPage';
import UsageDashboardPage from './pages/UsageDashboardPage';
import theme from './theme';

function App() {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter>
                <AuthProvider>
                    <Routes>
                        <Route path="/login" element={<Login />} />
                        <Route
                            path="/*"
                            element={
                                <ProtectedRoute>
                                    <Layout>
                                        <Routes>
                                            <Route path="/" element={<Dashboard />} />
                                            <Route path="/home" element={<Home />} />
                                            {/* Function panel routes */}
                                            <Route path="/use-openai" element={<UseOpenAIPage />} />
                                            <Route path="/use-anthropic" element={<UseAnthropicPage />} />
                                            <Route path="/use-claude-code" element={<UseClaudeCodePage />} />
                                            {/* Other routes */}
                                            <Route path="/api-keys" element={<ApiKeyPage />} />
                                            <Route path="/oauth" element={<OAuthPage />} />
                                            <Route path="/system" element={<System />} />
                                            <Route path="/usage" element={<UsageDashboardPage />} />
                                            <Route path="/model-test/:providerUuid" element={<ModelTestPage />} />
                                        </Routes>
                                    </Layout>
                                </ProtectedRoute>
                            }
                        />
                    </Routes>
                </AuthProvider>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
