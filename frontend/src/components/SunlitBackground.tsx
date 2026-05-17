import { useTheme } from '@mui/material/styles';
import { useThemeMode } from '../contexts/ThemeContext';
import { useEffect, useRef, useCallback, useState } from 'react';
import { Z_INDEX } from '../constants/zIndex';

const LEAVES_IMAGE_PATH = '/assets/leaves.png';

/**
 * Sunlit theme background component
 * Uses CSS for gradient (GPU-accelerated) and canvas only for leaves overlay
 */
export const SunlitBackground: React.FC = () => {
    const theme = useTheme();
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const { mode } = useThemeMode();
    const isDark = mode === 'dark';
    const animationFrameRef = useRef<number | undefined>(undefined);
    const [leavesImage, setLeavesImage] = useState<HTMLImageElement | null>(null);

    // Preload leaves image once
    useEffect(() => {
        const img = new Image();
        img.onload = () => setLeavesImage(img);
        img.src = LEAVES_IMAGE_PATH;
    }, []);

    // Get gradient colors from theme
    const gradientColors = (theme.palette.background as any).gradient;

    const renderLeaves = useCallback(() => {
        const canvas = canvasRef.current;
        if (!canvas || !leavesImage) return;

        const ctx = canvas.getContext('2d', { alpha: true });
        if (!ctx) return;

        const dpr = window.devicePixelRatio || 1;
        const w = window.innerWidth;
        const h = window.innerHeight;

        // Set canvas size with device pixel ratio for crisp rendering
        canvas.width = w * dpr;
        canvas.height = h * dpr;
        ctx.scale(dpr, dpr);

        // Clear canvas
        ctx.clearRect(0, 0, w, h);

        // Position leaves in bottom-right corner
        const leafSize = Math.min(w, h) * 0.75;
        const leafX = w - leafSize;
        const leafY = h - leafSize * 0.65;

        ctx.save();

        // Draw soft shadow behind leaves
        ctx.globalAlpha = isDark ? 0.25 : 0.15;
        ctx.filter = 'blur(32px)';
        ctx.drawImage(
            leavesImage,
            leafX + 16,
            leafY + 16,
            leafSize,
            leafSize * 0.7
        );

        // Draw leaves with subtle transparency
        ctx.filter = 'none';
        ctx.globalAlpha = isDark ? 0.35 : 0.22;
        ctx.drawImage(
            leavesImage,
            leafX,
            leafY,
            leafSize,
            leafSize * 0.7
        );

        ctx.restore();
    }, [leavesImage, isDark]);

    // Handle resize with debouncing
    useEffect(() => {
        if (!leavesImage) return;

        const handleResize = () => {
            if (animationFrameRef.current) {
                cancelAnimationFrame(animationFrameRef.current);
            }
            animationFrameRef.current = requestAnimationFrame(renderLeaves);
        };

        // Initial render
        renderLeaves();
        window.addEventListener('resize', handleResize);

        return () => {
            window.removeEventListener('resize', handleResize);
            if (animationFrameRef.current) {
                cancelAnimationFrame(animationFrameRef.current);
            }
        };
    }, [leavesImage, renderLeaves]);

    // CSS gradient background - GPU accelerated, no canvas needed
    const gradientStyle = isDark
        ? `linear-gradient(135deg, ${theme.palette.background.default} 0%, ${theme.palette.background.paper} 100%)`
        : `linear-gradient(135deg, ${gradientColors?.start || '#e0f2fe'} 0%, ${gradientColors?.middle || '#bae6fd'} 50%, ${gradientColors?.end || '#7dd3fc'} 100%)`;

    return (
        <>
            {/* CSS gradient layer - GPU accelerated */}
            <div
                style={{
                    position: 'fixed',
                    top: 0,
                    left: 0,
                    width: '100vw',
                    height: '100vh',
                    background: gradientStyle,
                    pointerEvents: 'none',
                    zIndex: Z_INDEX.sunlitBackground,
                }}
            />
            {/* Canvas layer for leaves only */}
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
        </>
    );
};
