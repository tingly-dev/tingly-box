package protocolserver

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/vision/imagegen"
)

// checkImageOutputCount rejects n above the GIF cap: each animated image is a
// slow, costly upstream call.
func checkImageOutputCount(opts imagegen.OutputOptions, n int64) error {
	if opts.IsGIF() && n > imagegen.MaxGIFImages {
		return fmt.Errorf("n must be at most %d for output_format gif", imagegen.MaxGIFImages)
	}
	return nil
}

// checkImageOutputSupport refuses an output format the selected provider
// cannot produce, instead of letting it answer with a still image.
func checkImageOutputSupport(provider *typ.Provider, opts imagegen.OutputOptions, edit bool) error {
	if opts.Format == "" || imagegen.DetectVendor(provider).SupportsOutputFormat(opts.Format, edit) {
		return nil
	}
	name := ""
	if provider != nil {
		name = provider.Name
	}
	op := "image generation"
	if edit {
		op = "image edit"
	}
	return fmt.Errorf("output_format %s is not supported by provider %q for %s (animated GIF is experimental and available through Codex)", opts.Format, name, op)
}
