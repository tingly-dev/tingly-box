import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import ProtectedRoute from './components/ProtectedRoute';
import { AuthProvider } from './contexts/AuthContext';
import Layout from './layout/Layout';
import Home from './pages/Home.tsx';
import Login from './pages/Login';
import ApiKeyPage from './pages/ApiKeyPage.tsx';
import OAuthPage from './pages/OAuthPage.tsx';
import RulePage from './pages/RulePage.tsx';
import System from './pages/System';
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
                                            <Route path="/" element={<Home />} />
                                            <Route path="/home" element={<Home />} />
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
