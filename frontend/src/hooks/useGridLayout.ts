import {useEffect, useState} from 'react';

interface GridLayout {
    columns: number;
    rows: number;
    modelsPerPage: number;
    cardWidth: string;
}

export const useGridLayout = () => {
    const calculateGridLayout = (): GridLayout => {
        const viewportWidth = window.innerWidth;
        const viewportHeight = window.innerHeight;

        // Reserve space for UI elements (header, tabs, search, pagination, etc.)
        const headerHeight = 200; // Approximate height for headers, tabs, search, etc.
        const availableHeight = viewportHeight - headerHeight;

        // Card dimensions including gaps
        const cardWidth = 160; // Increased from 140 to 160
        const cardHeight = 100; // Increased from 80 to 100
        const minGap = 8;

        // Calculate columns based on viewport width
        const maxColumns = Math.floor((viewportWidth - 100) / (cardWidth + minGap)); // Reserve 100px for padding
        const columns = Math.max(3, Math.min(6, maxColumns)); // Between 3-8 columns

        // Calculate rows based on available height
        const maxRows = Math.floor(availableHeight / cardHeight);
        const rows = Math.min(8, maxRows); // Increased from 2 to 4 rows

        const modelsPerPage = columns * rows;

        return {
            columns,
            rows,
            modelsPerPage: Math.max(12, Math.min(48, modelsPerPage)), // Reduced to 12-24 models per page
            cardWidth: `${100 / columns}%` // Responsive width
        };
    };

    const [gridLayout, setGridLayout] = useState<GridLayout>(calculateGridLayout());

    // Update grid layout when window resizes
    useEffect(() => {
        const handleResize = () => {
            const newLayout = calculateGridLayout();
            setGridLayout(newLayout);
        };

        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, []);

    return gridLayout;
};