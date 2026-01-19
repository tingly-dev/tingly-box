import { useEffect, useRef, useState } from 'react';
import styled from '@emotion/styled';

const BackgroundContainer = styled.div`
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  z-index: -1;
  overflow: hidden;
  background: #0a0a0f;
`;

const Canvas = styled.canvas`
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
`;

interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  targetRadius: number;
  maxRadius: number;
  minRadius: number;
  pulseSpeed: number;
  pulsePhase: number;
  color: string;
  baseAlpha: number;
}

const AnimatedBackground = () => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const particlesRef = useRef<Particle[]>([]);
  const mouseRef = useRef({ x: 0, y: 0 });
  const lastTimeRef = useRef<number>(0);
  const bgCanvasRef = useRef<HTMLCanvasElement | null>(null);
  const bgCtxRef = useRef<CanvasRenderingContext2D | null>(null);
  const isVisibleRef = useRef(true);
  const [isInitialized, setIsInitialized] = useState(false);
  const animationIdRef = useRef<number>();

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d', {
      alpha: false,
      willReadFrequently: false,
      desynchronized: true
    });
    if (!ctx) return;

    // Set canvas size with device pixel ratio for sharper rendering
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    const resizeCanvas = () => {
      const width = window.innerWidth;
      const height = window.innerHeight;

      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = width + 'px';
      canvas.style.height = height + 'px';

      ctx.scale(dpr, dpr);

      // Update offscreen canvas for background
      createBackgroundCanvas(width, height);
    };

    // Create offscreen canvas for static background
    const createBackgroundCanvas = (width: number, height: number) => {
      if (!bgCanvasRef.current) {
        bgCanvasRef.current = document.createElement('canvas');
        bgCtxRef.current = bgCanvasRef.current.getContext('2d', { alpha: false });
      }

      if (bgCanvasRef.current && bgCtxRef.current) {
        bgCanvasRef.current.width = width;
        bgCanvasRef.current.height = height;

        // Create gradient background once
        const bgGradient = bgCtxRef.current.createRadialGradient(
          width / 2,
          height / 2,
          0,
          width / 2,
          height / 2,
          Math.max(width, height) / 2
        );
        bgGradient.addColorStop(0, 'rgba(18, 18, 30, 0.85)');
        bgGradient.addColorStop(0.5, 'rgba(12, 12, 22, 0.92)');
        bgGradient.addColorStop(1, 'rgba(5, 5, 15, 0.98)');

        bgCtxRef.current.fillStyle = bgGradient;
        bgCtxRef.current.fillRect(0, 0, width, height);
      }
    };

    resizeCanvas();
    let resizeTimeout: number;
    const handleResize = () => {
      clearTimeout(resizeTimeout);
      resizeTimeout = window.setTimeout(resizeCanvas, 250);
    };
    window.addEventListener('resize', handleResize, { passive: true });

    // Enhanced color palette for better visibility
    const colors = [
      { r: 168, g: 85, b: 247 },   // Vibrant Violet
      { r: 99, g: 102, b: 241 },   // Bright Indigo
      { r: 59, g: 130, b: 246 },   // Strong Blue
      { r: 34, g: 211, b: 238 },   // Bright Cyan
      { r: 139, g: 92, b: 246 },   // Purple
      { r: 6, g: 182, b: 212 },    // Light Blue
    ];

    // Initialize particles with performance optimizations
    const initParticles = () => {
      // Responsive particle count based on screen size
      const isMobile = window.innerWidth < 768;
      const baseCount = isMobile ? 15 : 35;
      const areaFactor = window.innerWidth * window.innerHeight;
      const divisor = isMobile ? 35000 : 25000;
      const particleCount = Math.min(baseCount, Math.floor(areaFactor / divisor));
      const particles: Particle[] = [];

      for (let i = 0; i < particleCount; i++) {
        const color = colors[Math.floor(Math.random() * colors.length)];
        // Smaller particles on mobile
        const maxRadius = isMobile ? Math.random() * 150 + 120 : Math.random() * 220 + 180;
        const minRadius = isMobile ? Math.random() * 25 + 25 : Math.random() * 40 + 40;

        // Higher brightness on desktop
        const baseAlphaRange = isMobile ? { min: 0.05, max: 0.12 } : { min: 0.08, max: 0.18 };
        const baseAlpha = Math.random() * (baseAlphaRange.max - baseAlphaRange.min) + baseAlphaRange.min;

        particles.push({
          x: Math.random() * window.innerWidth,
          y: Math.random() * window.innerHeight,
          vx: (Math.random() - 0.5) * 0.5,
          vy: (Math.random() - 0.5) * 0.5,
          radius: minRadius,
          targetRadius: minRadius,
          maxRadius,
          minRadius,
          color: `rgba(${color.r}, ${color.g}, ${color.b}`,
          baseAlpha,
          pulseSpeed: Math.random() * 0.008 + 0.005,
          pulsePhase: Math.random() * Math.PI * 2,
        });
      }

      particlesRef.current = particles;
    };

    initParticles();

    // Mouse move handler with throttling
    let mouseUpdateTimeout: number;
    const handleMouseMove = (e: MouseEvent) => {
      if (mouseUpdateTimeout) return;
      mouseUpdateTimeout = requestAnimationFrame(() => {
        mouseRef.current = { x: e.clientX, y: e.clientY };
        mouseUpdateTimeout = 0;
      });
    };

    window.addEventListener('mousemove', handleMouseMove, { passive: true });

    // Visibility API to pause animation when tab is not visible
    const handleVisibilityChange = () => {
      isVisibleRef.current = document.visibilityState === 'visible';
      if (isVisibleRef.current && !animationIdRef.current) {
        lastTimeRef.current = performance.now();
        animate(lastTimeRef.current);
      }
    };

    window.addEventListener('visibilitychange', handleVisibilityChange, { passive: true });

    // Check for reduced motion preference
    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const motionFactor = prefersReducedMotion ? 0.1 : 1;

    // Animation loop with performance optimizations
    const animate = (currentTime: number) => {
      if (!isVisibleRef.current) {
        animationIdRef.current = 0;
        return;
      }

      // Calculate delta time for frame-rate independent animation
      const deltaTime = currentTime - lastTimeRef.current;
      lastTimeRef.current = currentTime;

      // Skip frames if delta is too small to save resources
      if (deltaTime < 16) { // Maintain stable 60fps
        animationIdRef.current = requestAnimationFrame(animate);
        return;
      }

      // Clamp delta to prevent large jumps
      const deltaFactor = Math.min(Math.max(deltaTime / 16.67, 0.5), 1.5); // Clamp between 0.5x and 1.5x speed

      // Clear canvas with more opaque fill to prevent white buildup
      ctx.fillStyle = 'rgba(10, 10, 15, 0.4)';
      ctx.fillRect(0, 0, window.innerWidth, window.innerHeight);

      // Skip the additional background gradient to reduce whiteness

      const mouseX = mouseRef.current.x;
      const mouseY = mouseRef.current.y;
      const particles = particlesRef.current;

      // Update and draw particles
      for (let i = 0; i < particles.length; i++) {
        const particle = particles[i];

        // Update pulse
        particle.pulsePhase += particle.pulseSpeed * deltaFactor * motionFactor;
        const pulseFactor = Math.sin(particle.pulsePhase) * 0.5 + 0.5;
        particle.targetRadius = particle.minRadius + (particle.maxRadius - particle.minRadius) * pulseFactor;

        // Smooth radius transitions - make it slower to reduce flicker
        particle.radius += (particle.targetRadius - particle.radius) * 0.06;

        // Update position with delta time
        particle.x += particle.vx * deltaFactor * motionFactor * 0.8;
        particle.y += particle.vy * deltaFactor * motionFactor * 0.8;

        // Mouse interaction with optimized distance calculation
        const dx = mouseX - particle.x;
        const dy = mouseY - particle.y;
        const distSq = dx * dx + dy * dy;
        // Smaller interaction radius on mobile
        const effectRadius = window.innerWidth < 768 ? 200 : 300;
        const effectRadiusSq = effectRadius * effectRadius;

        if (distSq < effectRadiusSq) {
          const dist = Math.sqrt(distSq);
          const force = (effectRadius - dist) / effectRadius * 0.035;
          particle.vx += (dx / dist) * force * deltaFactor * 0.001;
          particle.vy += (dy / dist) * force * deltaFactor * 0.001;
        }

        // Apply damping
        particle.vx *= 0.99;
        particle.vy *= 0.99;

        // Boundary collision
        const canvasWidth = window.innerWidth;
        const canvasHeight = window.innerHeight;

        if (particle.x < particle.radius || particle.x > canvasWidth - particle.radius) {
          particle.vx = -particle.vx * 0.9;
          particle.x = particle.x < particle.radius ? particle.radius : canvasWidth - particle.radius;
        }
        if (particle.y < particle.radius || particle.y > canvasHeight - particle.radius) {
          particle.vy = -particle.vy * 0.9;
          particle.y = particle.y < particle.radius ? particle.radius : canvasHeight - particle.radius;
        }

        // Draw particle with simple glow effect
        const alpha = particle.baseAlpha * (0.3 + pulseFactor * 0.2);

        // Use 'lighter' composite operation which is more stable than 'screen'
        ctx.globalCompositeOperation = 'lighter';

        // Create simple radial gradient for glow
        const gradient = ctx.createRadialGradient(
          particle.x,
          particle.y,
          0,
          particle.x,
          particle.y,
          particle.radius * 1.2
        );

        gradient.addColorStop(0, particle.color + ', ' + alpha + ')');
        gradient.addColorStop(0.3, particle.color + ', ' + (alpha * 0.6) + ')');
        gradient.addColorStop(1, particle.color + ', 0)');

        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.arc(particle.x, particle.y, particle.radius * 1.2, 0, Math.PI * 2);
        ctx.fill();

        // Reset composite operation for next particle
        ctx.globalCompositeOperation = 'source-over';
      }

      animationIdRef.current = requestAnimationFrame(animate);
    };

    // Start animation
    lastTimeRef.current = performance.now();
    animate(lastTimeRef.current);
    setIsInitialized(true);

    // Cleanup
    return () => {
      window.removeEventListener('resize', handleResize);
      window.removeEventListener('mousemove', handleMouseMove);
      window.removeEventListener('visibilitychange', handleVisibilityChange);
      clearTimeout(resizeTimeout);
      if (mouseUpdateTimeout) {
        cancelAnimationFrame(mouseUpdateTimeout);
      }
      if (animationIdRef.current) {
        cancelAnimationFrame(animationIdRef.current);
      }
      if (bgCanvasRef.current) {
        bgCanvasRef.current = null;
        bgCtxRef.current = null;
      }
    };
  }, []);

  return (
    <BackgroundContainer>
      <Canvas ref={canvasRef} />
      {isInitialized && (
        <style jsx>{`
          @keyframes fadeIn {
            from { opacity: 0; }
            to { opacity: 1; }
          }
          canvas {
            animation: fadeIn 2s ease-out;
          }
        `}</style>
      )}
    </BackgroundContainer>
  );
};

export default AnimatedBackground;