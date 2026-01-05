#!/usr/bin/env python3
"""
Combine images into a GIF animation.
"""

import argparse
import os
import sys
from pathlib import Path

try:
    from PIL import Image, ImageDraw
except ImportError:
    print("Error: PIL/Pillow is not installed.")
    print("Install with: pip install Pillow")
    sys.exit(1)


def round_corners(img: Image.Image, radius: int = 120) -> Image.Image:
    """
    Apply rounded corners to an image.

    Args:
        img: Input image
        radius: Corner radius in pixels (default: 20px)

    Returns:
        Image with rounded corners
    """
    w, h = img.size

    # Create a mask for rounded corners
    mask = Image.new('L', (w, h), 0)
    draw = ImageDraw.Draw(mask)

    # Draw rounded rectangle (white on transparent)
    draw.rounded_rectangle(
        [0, 0, w, h],
        radius=radius,
        fill=255
    )

    # Create output image with transparent background
    output = Image.new('RGBA', (w, h), (255, 255, 255, 255))

    # Paste original image using the mask
    if img.mode != 'RGBA':
        img = img.convert('RGBA')
    output.paste(img, (0, 0), mask)

    return output.convert('RGB')


def create_gif(
    images: list[Path],
    output: Path,
    duration: int = 500,
    loop: int = 0,
    optimize: bool = True,
    resize: bool = True,
    fill_corners: bool = True,
    corner_size: int = 200,
):
    """
    Create a GIF from a list of images.

    Args:
        images: List of image file paths
        output: Output GIF file path
        duration: Duration per frame in milliseconds
        loop: Number of loops (0 = infinite loop)
        optimize: Whether to optimize the GIF
        resize: Whether to resize images to minimum dimensions
        fill_corners: Whether to round corners
        corner_size: Corner radius in pixels
    """
    if not images:
        raise ValueError("No images provided")

    # First pass: load images and find min dimensions
    frames = []
    sizes = []
    for img_path in images:
        try:
            img = Image.open(img_path)
            # Convert to RGB if necessary
            if img.mode not in ('RGB', 'RGBA'):
                img = img.convert('RGB')
            frames.append(img)
            sizes.append(img.size)
        except Exception as e:
            print(f"Warning: Could not load {img_path}: {e}")

    if not frames:
        raise ValueError("No valid images could be loaded")

    # Resize to minimum dimensions (top-left anchored)
    if resize and len(sizes) > 1:
        min_w = min(s[0] for s in sizes)
        min_h = min(s[1] for s in sizes)
        print(f"Resizing all images to {min_w}x{min_h} (minimum dimensions)")
        resized_frames = []
        for i, frame in enumerate(frames):
            if frame.size != (min_w, min_h):
                # Use NEAREST for pixel art, LANCZOS for photos
                frame = frame.resize((min_w, min_h), Image.Resampling.LANCZOS)
            resized_frames.append(frame)
        frames = resized_frames

    # Round corners
    if fill_corners:
        print(f"Rounding corners (radius: {corner_size}px)")
        frames = [round_corners(frame, corner_size) for frame in frames]

    # Save as GIF
    frames[0].save(
        output,
        format='GIF',
        append_images=frames[1:],
        save_all=True,
        duration=duration,
        loop=loop,
        optimize=optimize,
    )

    print(f"Created GIF: {output}")
    print(f"  Frames: {len(frames)}")
    print(f"  Size: {frames[0].size}")
    print(f"  Duration per frame: {duration}ms")


def main():
    parser = argparse.ArgumentParser(
        description="Combine images into a GIF animation"
    )
    parser.add_argument(
        "images",
        nargs="+",
        type=Path,
        help="Image files to combine (in order)",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        default=None,
        help="Output GIF file (default: output.gif)",
    )
    parser.add_argument(
        "-d",
        "--duration",
        type=int,
        default=500,
        help="Duration per frame in milliseconds (default: 500)",
    )
    parser.add_argument(
        "-l",
        "--loop",
        type=int,
        default=0,
        help="Number of loops (default: 0 = infinite)",
    )
    parser.add_argument(
        "--no-optimize",
        action="store_false",
        dest="optimize",
        help="Disable GIF optimization",
    )
    parser.add_argument(
        "--no-resize",
        action="store_false",
        dest="resize",
        help="Disable image resizing (keep original sizes)",
    )
    parser.add_argument(
        "--no-fill-corners",
        action="store_false",
        dest="fill_corners",
        help="Disable corner rounding",
    )
    parser.add_argument(
        "--corner-size",
        type=int,
        default=20,
        help="Corner radius in pixels (default: 20)",
    )

    args = parser.parse_args()

    # Determine output path
    output = args.output or Path("output.gif")

    create_gif(
        images=args.images,
        output=output,
        duration=args.duration,
        loop=args.loop,
        optimize=args.optimize,
        resize=args.resize,
        fill_corners=args.fill_corners,
        corner_size=args.corner_size,
    )


if __name__ == "__main__":
    main()
