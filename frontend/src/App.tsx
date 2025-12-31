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
import RulePage from './pages/RulePage';
import System from './pages/System';
import UseOpenAIPageWrapper from './pages/wrappers/UseOpenAIPageWrapper';
import UseAnthropicPageWrapper from './pages/wrappers/UseAnthropicPageWrapper';
import UseClaudeCodePageWrapper from './pages/wrappers/UseClaudeCodePageWrapper';
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
                                            <Route path="/use-openai" element={<UseOpenAIPageWrapper />} />
                                            <Route path="/use-anthropic" element={<UseAnthropicPageWrapper />} />
                                            <Route path="/use-claude-code" element={<UseClaudeCodePageWrapper />} />
                                            {/* Other routes */}
                                            <Route path="/api-keys" element={<ApiKeyPage />} />
                                            <Route path="/oauth" element={<OAuthPage />} />
                                            <Route path="/routing" element={<RulePage />} />
                                            <Route path="/system" element={<System />} />
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
