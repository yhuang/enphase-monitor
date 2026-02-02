#!/usr/bin/env bash

# Script to generate PDF files from markdown files in the root directory
# Requires pandoc: brew install pandoc (on macOS) or apt-get install pandoc (on Linux)

# Do not exit on error - we want to process all files even if one fails
set +e

# Check if pandoc is installed
if ! command -v pandoc &> /dev/null; then
    echo "ERROR: pandoc is not installed."
    echo ""
    echo "To install pandoc:"
    echo "  macOS:   brew install pandoc"
    echo "  Linux:   sudo apt-get install pandoc"
    echo "  Windows: Download from https://pandoc.org/installing.html"
    echo ""
    exit 1
fi

# Directory for PDF output
PDF_DIR="pdfs"
mkdir -p "$PDF_DIR"

# List of markdown files to convert
MD_FILES=(
    "README.md"
    "QUICKSTART.md"
    "docs/GO_BEST_PRACTICES.md"
)

echo "Generating PDF files from markdown..."
echo ""

# Convert each markdown file to PDF
for md_file in "${MD_FILES[@]}"; do
    if [ ! -f "$md_file" ]; then
        echo "⚠️  Warning: $md_file not found, skipping..."
        continue
    fi
    
    # Generate PDF filename (e.g., README.md -> README.pdf)
    pdf_file="${md_file%.md}.pdf"
    pdf_path="$PDF_DIR/$pdf_file"
    
    echo "Converting $md_file -> $pdf_path"
    
    # Convert markdown to PDF using pandoc
    # Pandoc will automatically use an available PDF engine
    # Common engines: pdflatex, xelatex, lualatex, wkhtmltopdf, weasyprint
    if pandoc "$md_file" \
        -o "$pdf_path" \
        -V geometry:margin=1in \
        -V fontsize=11pt \
        -V documentclass=article \
        --toc \
        --toc-depth=3 \
        --pdf-engine=xelatex \
        2>&1; then
        # Success
        :
    else
        echo "❌ Error: Failed to convert $md_file"
        echo "   Note: Pandoc requires a PDF engine to be installed."
        echo "   On macOS, install BasicTeX: brew install --cask basictex"
        echo "   Or install MacTeX for full LaTeX support: brew install --cask mactex"
        echo "   After installing, you may need to add /Library/TeX/texbin to your PATH"
        continue
    fi
    
    echo "✅ Created: $pdf_path"
    echo ""
done

echo "PDF generation complete! PDFs are in the '$PDF_DIR' directory."
ls -lh "$PDF_DIR"/*.pdf 2>/dev/null || echo "No PDF files were generated."
