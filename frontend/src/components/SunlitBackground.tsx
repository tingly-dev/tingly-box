import { useThemeMode } from '../contexts/ThemeContext';
import { useEffect, useState, useMemo } from 'react';
import { Z_INDEX } from '../constants/zIndex';

/**
 * Sunlit theme background component - Performance optimized
 * Inspired by https://github.com/ugur-eren/sunlit
 * Features: dappled light, blinds, progressive blur, leaf billowing effect
 */
export const SunlitBackground: React.FC = () => {
    const { mode } = useThemeMode();
    const [animationReady, setAnimationReady] = useState(false);
    const [reduceMotion, setReduceMotion] = useState(false);
    const isDark = mode === 'dark';

    useEffect(() => {
        // Check user's motion preference
        const mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
        setReduceMotion(mediaQuery.matches);

        const handleChange = () => setReduceMotion(mediaQuery.matches);
        mediaQuery.addEventListener('change', handleChange);

        // Trigger animation after mount
        const timer = setTimeout(() => setAnimationReady(true), 50);
        return () => {
            clearTimeout(timer);
            mediaQuery.removeEventListener('change', handleChange);
        };
    }, []);

    // Memoize static elements to avoid re-renders
    const blinds = useMemo(() => {
        // Reduce number of blinds for better performance
        const count = reduceMotion ? 12 : 16;
        return Array.from({ length: count }).map((_, i) => (
            <div
                key={i}
                style={{
                    width: '100%',
                    height: isDark ? '80px' : '40px',
                    backgroundColor: isDark ? '#030307' : '#1a1917',
                    transition: 'height 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                    willChange: 'auto', // Disable will-change for better performance
                }}
            />
        ));
    }, [isDark, reduceMotion]);

    const verticalBars = useMemo(() => {
        return Array.from({ length: 2 }).map((_, i) => ( // Reduce from 3 to 2
            <div
                key={i}
                style={{
                    width: '5px',
                    height: '100%',
                    backgroundColor: isDark ? '#030307' : '#1a1917',
                }}
            />
        ));
    }, [isDark]);

    // Use CSS containment for better performance
    const containerStyle = {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        width: '100vw',
        height: '100vh',
        pointerEvents: 'none' as const,
        zIndex: Z_INDEX.sunlitBackground,
        opacity: animationReady ? 1 : 0,
        transition: 'opacity 0.5s ease',
        contain: 'strict', // CSS containment for performance
        willChange: 'auto', // Disable will-change optimization
    };

    // Reduce blur layers for better performance
    const blurLayers = useMemo(() => {
        if (reduceMotion) {
            // Only 2 blur layers when motion is reduced
            return [
                {
                    blur: '8px',
                    mask: 'linear-gradient(252deg, transparent 0%, transparent 50%, black 100%)',
                },
                {
                    blur: '24px',
                    mask: 'linear-gradient(252deg, transparent 0%, black 100%)',
                },
            ];
        }
        // 3 layers instead of 4 for normal mode
        return [
            {
                blur: '8px',
                mask: 'linear-gradient(252deg, transparent, transparent 0%, black 0%, black)',
            },
            {
                blur: '24px',
                mask: 'linear-gradient(252deg, transparent, transparent 40%, black 80%, black)',
            },
            {
                blur: '64px',
                mask: 'linear-gradient(252deg, transparent, transparent 60%, black 80%, black)',
            },
        ];
    }, [reduceMotion]);

    return (
        <div style={containerStyle}>
            {/* Glow effect */}
            <div
                style={{
                    position: 'absolute',
                    background: isDark
                        ? 'linear-gradient(355deg, #1b293f 0%, transparent 30%, transparent 100%)'
                        : 'linear-gradient(309deg, #f5d7a6, #f5d7a6 20%, transparent)',
                    transition: 'background 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                    height: '100%',
                    width: '100%',
                    opacity: isDark ? 0.5 : 0.4,
                }}
            />

            {/* Blinds effect */}
            <div
                style={{
                    position: 'absolute',
                    top: '-30vh',
                    right: 0,
                    width: '80vw',
                    height: '130vh',
                    opacity: isDark ? 0.25 : 0.12,
                    backgroundBlendMode: 'darken',
                    transformOrigin: 'top right',
                    transform: isDark
                        ? 'matrix3d(0.8333, 0.0833, 0.0000, 0.0003, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000)'
                        : 'matrix3d(0.7500, -0.0625, 0.0000, 0.0008, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000, 0.0000, 0.0000, 0.0000, 0.0000, 1.0000)',
                    transition: 'transform 1.7s cubic-bezier(0.455, 0.190, 0.000, 0.985), opacity 4s ease',
                    contain: 'strict', // CSS containment
                    willChange: 'auto',
                }}
            >
                {/* Leaves with subtle animation - only when not reduced motion */}
                {!reduceMotion && (
                    <div
                        style={{
                            position: 'absolute',
                            backgroundSize: 'cover',
                            backgroundRepeat: 'no-repeat',
                            bottom: '-20px',
                            right: '-700px',
                            width: '1600px',
                            height: '1400px',
                            backgroundImage: 'url("/assets/leaves.png")',
                            opacity: 0.5, // Reduced opacity
                            animation: 'leafBillow 8s ease-in-out infinite',
                        }}
                    />
                )}

                {/* Horizontal blinds */}
                <div
                    style={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'flex-end',
                        gap: isDark ? '20px' : '60px',
                        transition: 'gap 1.0s cubic-bezier(0.455, 0.190, 0.000, 0.985)',
                        width: '100%',
                    }}
                >
                    {blinds}
                </div>

                {/* Vertical bars */}
                <div
                    style={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        width: '100%',
                        height: '100%',
                        display: 'flex',
                        justifyContent: 'space-around',
                    }}
                >
                    {verticalBars}
                </div>
            </div>

            {/* Progressive blur layers - optimized */}
            <div
                style={{
                    position: 'absolute',
                    height: '100%',
                    width: '100%',
                    contain: 'strict',
                }}
            >
                {blurLayers.map((layer, i) => (
                    <div
                        key={i}
                        style={{
                            position: 'absolute',
                            height: '100%',
                            width: '100%',
                            inset: 0,
                            backdropFilter: `blur(${layer.blur})`,
                            maskImage: layer.mask,
                        }}
                    />
                ))}
            </div>

            <style>{`
                @keyframes leafBillow {
                    0% {
                        transform: perspective(400px) rotateX(0deg) rotateY(0deg) scale(1);
                    }
                    25% {
                        transform: perspective(400px) rotateX(1deg) rotateY(2deg) scale(1.02);
                    }
                    50% {
                        transform: perspective(400px) rotateX(-4deg) rotateY(-2deg) scale(0.97);
                    }
                    75% {
                        transform: perspective(400px) rotateX(1deg) rotateY(-1deg) scale(1.04);
                    }
                    100% {
                        transform: perspective(400px) rotateX(0deg) rotateY(0deg) scale(1);
                    }
                }
            `}</style>
        </div>
    );
};
