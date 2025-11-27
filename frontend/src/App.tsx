import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import { BrowserRouter, Route, Routes } from 'react-router-dom';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Providers from './pages/Providers';
import Server from './pages/Server';
import theme from './theme';

function App() {
    return (
        <ThemeProvider theme={theme}>
            <CssBaseline />
            <BrowserRouter>
                <Layout>
                    <Routes>
                        <Route path="/" element={<Dashboard />} />
                        <Route path="/providers" element={<Providers />} />
                        <Route path="/server" element={<Server />} />
                        {/* <Route path="/history" element={<History />} /> */}
                    </Routes>
                </Layout>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
