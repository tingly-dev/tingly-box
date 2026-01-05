#!/bin/bash
# Create GIF from images in this directory

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Default values
OUTPUT="output.gif"
DURATION=500

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -o|--output)
            OUTPUT="$2"
            shift 2
            ;;
        -d|--duration)
            DURATION="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [-o OUTPUT] [-d DURATION]"
            echo "  -o, --output    Output GIF filename (default: output.gif)"
            echo "  -d, --duration  Duration per frame in ms (default: 500)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check Python and Pillow
if ! command -v python3 &> /dev/null; then
    echo "Error: python3 is not installed"
    exit 1
fi

if ! python3 -c "import PIL" 2>/dev/null; then
    echo "Error: Pillow is not installed"
    echo "Install with: pip install Pillow"
    exit 1
fi

# Get all PNG files in alphabetical order
IMAGES=($(ls -1 *.png 2>/dev/null))

if [ ${#IMAGES[@]} -eq 0 ]; then
    echo "Error: No PNG files found in $SCRIPT_DIR"
    exit 1
fi

echo "Creating GIF from ${#IMAGES[@]} images:"
printf '  %s\n' "${IMAGES[@]}"
echo

python3 create_gif.py "${IMAGES[@]}" -o "$OUTPUT" -d "$DURATION"
