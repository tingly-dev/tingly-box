package imagegen

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/vision/imagefmt"
)

func TestParseOutputOptionsJSON(t *testing.T) {
	t.Run("absent fields mean upstream defaults", func(t *testing.T) {
		o, err := ParseOutputOptionsJSON([]byte(`{"model":"m","prompt":"p"}`))
		require.NoError(t, err)
		assert.Equal(t, OutputOptions{}, o)
		assert.False(t, o.IsGIF())
	})
	t.Run("gif with frame controls", func(t *testing.T) {
		o, err := ParseOutputOptionsJSON([]byte(`{"output_format":"gif","frame_count":5,"frame_duration_ms":120}`))
		require.NoError(t, err)
		assert.Equal(t, OutputOptions{Format: imagefmt.GIF, FrameCount: 5, FrameDurationMS: 120}, o)
		assert.True(t, o.IsGIF())
	})
	t.Run("gif with defaults", func(t *testing.T) {
		o, err := ParseOutputOptionsJSON([]byte(`{"output_format":"GIF"}`))
		require.NoError(t, err)
		assert.Equal(t, OutputOptions{Format: imagefmt.GIF}, o)
	})
	t.Run("raster formats", func(t *testing.T) {
		for in, want := range map[string]imagefmt.Format{"png": imagefmt.PNG, "jpeg": imagefmt.JPEG, "jpg": imagefmt.JPEG, "webp": imagefmt.WebP} {
			o, err := ParseOutputOptionsJSON([]byte(`{"output_format":"` + in + `"}`))
			require.NoError(t, err, in)
			assert.Equal(t, want, o.Format, in)
		}
	})
	t.Run("rejections", func(t *testing.T) {
		for name, body := range map[string]string{
			"svg":                        `{"output_format":"svg"}`,
			"unknown":                    `{"output_format":"tiff"}`,
			"frames without gif":         `{"frame_count":5}`,
			"frames with png":            `{"output_format":"png","frame_count":5}`,
			"duration without gif":       `{"frame_duration_ms":100}`,
			"too few frames":             `{"output_format":"gif","frame_count":1}`,
			"too many frames":            `{"output_format":"gif","frame_count":17}`,
			"duration too short":         `{"output_format":"gif","frame_duration_ms":19}`,
			"duration too long":          `{"output_format":"gif","frame_duration_ms":2001}`,
			"fractional frames":          `{"output_format":"gif","frame_count":5.5}`,
			"string frames":              `{"output_format":"gif","frame_count":"5"}`,
			"output_format not a string": `{"output_format":3}`,
		} {
			_, err := ParseOutputOptionsJSON([]byte(body))
			require.Error(t, err, name)
			var oe *OutputOptionError
			assert.ErrorAs(t, err, &oe, name)
		}
	})
	t.Run("boundaries are accepted", func(t *testing.T) {
		for _, body := range []string{
			`{"output_format":"gif","frame_count":2,"frame_duration_ms":20}`,
			`{"output_format":"gif","frame_count":16,"frame_duration_ms":2000}`,
		} {
			_, err := ParseOutputOptionsJSON([]byte(body))
			require.NoError(t, err, body)
		}
	})
	t.Run("unparseable body is not this function's concern", func(t *testing.T) {
		o, err := ParseOutputOptionsJSON([]byte(`not json`))
		require.NoError(t, err)
		assert.Equal(t, OutputOptions{}, o)
	})
}

func TestParseOutputOptionsForm(t *testing.T) {
	o, err := ParseOutputOptionsForm("gif", "9", "200")
	require.NoError(t, err)
	assert.Equal(t, OutputOptions{Format: imagefmt.GIF, FrameCount: 9, FrameDurationMS: 200}, o)

	o, err = ParseOutputOptionsForm("", "", "")
	require.NoError(t, err)
	assert.Equal(t, OutputOptions{}, o)

	for _, c := range [][3]string{{"gif", "x", ""}, {"gif", "", "1.5"}, {"svg", "", ""}, {"png", "3", ""}, {"gif", "99", ""}} {
		_, err := ParseOutputOptionsForm(c[0], c[1], c[2])
		assert.Error(t, err, c)
	}
}

func TestVendorSupportsOutputFormat(t *testing.T) {
	type row struct {
		v       Vendor
		png     bool
		gifGen  bool
		gifEdit bool
		jpegEtc bool
	}
	for _, r := range []row{
		{VendorCodex, true, true, true, true},
		{VendorOpenAICompat, true, true, false, true},
		{VendorDashScope, true, false, false, true},
		{VendorMinimax, true, false, false, true},
		{VendorXAI, true, false, false, true},
		{VendorQianfan, true, false, false, true},
		{VendorUnknown, true, false, false, true},
	} {
		assert.Equal(t, r.png, r.v.SupportsOutputFormat(imagefmt.PNG, false), "%s png", r.v)
		assert.Equal(t, r.jpegEtc, r.v.SupportsOutputFormat(imagefmt.JPEG, false), "%s jpeg", r.v)
		assert.Equal(t, r.gifGen, r.v.SupportsOutputFormat(imagefmt.GIF, false), "%s gif generate", r.v)
		assert.Equal(t, r.gifEdit, r.v.SupportsOutputFormat(imagefmt.GIF, true), "%s gif edit", r.v)
	}
}

func TestOutputOptionsContext(t *testing.T) {
	assert.Equal(t, OutputOptions{}, OutputOptionsFrom(context.Background()))
	ctx := WithOutputOptions(context.Background(), OutputOptions{Format: imagefmt.GIF, FrameCount: 4})
	assert.Equal(t, OutputOptions{Format: imagefmt.GIF, FrameCount: 4}, OutputOptionsFrom(ctx))
}
