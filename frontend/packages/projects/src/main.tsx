import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
// Bundled Latin typeface (see fontSans in @tingly/ui/theme/fonts).
import '@fontsource-variable/inter/wght.css';
import './i18n';
import App from './App';

createRoot(document.getElementById('root')!).render(
    <StrictMode>
        <App />
    </StrictMode>,
);
