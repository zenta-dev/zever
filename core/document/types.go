package document

// OutputFormat represents a document render output format.
type OutputFormat string

// Supported document output formats.
const (
	// FormatPDF renders documents as PDF.
	FormatPDF OutputFormat = "pdf"
	// FormatPNG renders documents as PNG.
	FormatPNG OutputFormat = "png"
	// FormatJPG renders documents as JPG.
	FormatJPG OutputFormat = "jpg"
)
