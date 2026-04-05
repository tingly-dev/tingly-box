import { useTheme } from '@mui/material/styles';
import { useThemeMode } from '../contexts/ThemeContext';
import { useEffect, useRef } from 'react';
import { Z_INDEX } from '../constants/zIndex';

/**
 * Sunlit theme background component - Simple leaf shadows in corner
 * Minimal overhead, just leaves with subtle shadows
 */
export const SunlitBackground: React.FC = () => {
    const theme = useTheme();
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const { mode } = useThemeMode();
    const isDark = mode === 'dark';
    const animationFrameRef = useRef<number>(undefined);

    const renderCanvas = () => {
        const canvas = canvasRef.current;
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        if (!ctx) return;

        const w = canvas.width = window.innerWidth;
        const h = canvas.height = window.innerHeight;

        // Clear canvas
        ctx.clearRect(0, 0, w, h);

        // Draw warm base background
        const bgGradient = ctx.createLinearGradient(0, 0, w, h);
        if (isDark) {
            bgGradient.addColorStop(0, '#0f172a');
            bgGradient.addColorStop(1, '#1e293b');
        } else {
            // Read gradient colors from theme palette
            const gradientColors = (theme.palette.background as any).gradient;
            bgGradient.addColorStop(0, gradientColors.start);
            bgGradient.addColorStop(0.5, gradientColors.middle);
            bgGradient.addColorStop(1, gradientColors.end);
        }
        ctx.fillStyle = bgGradient;
        ctx.fillRect(0, 0, w, h);

        // Draw leaves in corner with shadow effect
        const img = new Image();
        img.onload = () => {
            ctx.save();

            // Position leaves in bottom-right corner
            const leafSize = Math.min(w, h) * 0.8;
            const leafX = w - leafSize;
            const leafY = h - leafSize * 0.7;

            // Draw shadow/blur effect behind leaves
            ctx.globalAlpha = isDark ? 0.3 : 0.2;
            ctx.filter = 'blur(40px)';

            // Shadow offset
            ctx.drawImage(
                img,
                leafX + 20,
                leafY + 20,
                leafSize,
                leafSize * 0.7
            );

            ctx.filter = 'none';
            ctx.globalAlpha = isDark ? 0.4 : 0.25;

            // Draw actual leaves
            ctx.drawImage(
                img,
                leafX,
                leafY,
                leafSize,
                leafSize * 0.7
            );

            ctx.restore();
        };
        img.onerror = () => {
            // If leaves fail to load, just draw background gradient
        };
        img.src = '/assets/leaves.png';
    };

    // Handle resize with debouncing
    useEffect(() => {
        const handleResize = () => {
            if (animationFrameRef.current) {
                cancelAnimationFrame(animationFrameRef.current);
            }
            animationFrameRef.current = requestAnimationFrame(renderCanvas);
        };

        const timer = setTimeout(renderCanvas, 50);
        window.addEventListener('resize', handleResize);

        return () => {
            clearTimeout(timer);
            window.removeEventListener('resize', handleResize);
            if (animationFrameRef.current) {
                cancelAnimationFrame(animationFrameRef.current);
            }
        };
    }, [isDark, theme]);

    return (
        <canvas
            ref={canvasRef}
            style={{
                position: 'fixed',
                top: 0,
                left: 0,
                width: '100vw',
                height: '100vh',
                pointerEvents: 'none',
                zIndex: Z_INDEX.sunlitBackground,
            }}
        />
    );
};
