package client

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/sirupsen/logrus"
)

// Experimental: image edit over the Responses hosted image_generation tool.
//
// The Codex-native images/edits endpoint (codex_images.go) has no mask field —
// verified against openai/codex source, see .design/image-mask.md §2.1 — so a
// masked edit cannot be honored there and must not be silently downgraded into
// a full repaint. The Responses hosted `image_generation` tool does carry one
// (`input_image_mask`, plus `action: edit`), and the same ChatGPT backend
// already serves our generation path through that tool, so this file routes
// masked edits there instead:
//
//	input: [ message(user, [input_text(prompt), input_image(data URL) ...]) ]
//	tools: [ image_generation{ action: "edit", input_image_mask: {image_url} } ]
//
// Whether that backend actually honors action/mask on this surface is the open
// question (.design/image-mask.md §2.5, experiments E1/E2) — this code is what
// makes those experiments runnable against a real subscription. Until they
// pass, the no-mask path keeps using the proven native endpoint.

// codexImageEditRoute selects which Codex surface serves an /images/edits call.
type codexImageEditRoute string

const (
	codexImageEditRouteNative    codexImageEditRoute = "native"
	codexImageEditRouteResponses codexImageEditRoute = "responses"
)

// codexImageEditRouteEnv forces a route for experiments: "responses" sends
// every Codex edit through the Responses tool (E1: does it take a reference
// image at all, and E3: quality against the native endpoint), "native" pins
// the old behavior so a masked request can be shown being dropped. Unset is
// the shipping behavior: mask decides.
const codexImageEditRouteEnv = "TINGLY_CODEX_IMAGE_EDIT_ROUTE"

// resolveCodexImageEditRoute picks the surface for one request. A mask is the
// only functional reason to leave the native endpoint, so it is the only
// automatic trigger; the env var exists for the experiments above and is
// expected to disappear once they settle.
func resolveCodexImageEditRoute(hasMask bool) codexImageEditRoute {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(codexImageEditRouteEnv))) {
	case string(codexImageEditRouteResponses):
		return codexImageEditRouteResponses
	case string(codexImageEditRouteNative):
		return codexImageEditRouteNative
	}
	if hasMask {
		return codexImageEditRouteResponses
	}
	return codexImageEditRouteNative
}

// imagesEditViaResponses serves an edit through the Responses image_generation
// tool. Response parsing is shared with the generation path — the tool emits
// the same image_generation_call events either way.
func (c *CodexClient) imagesEditViaResponses(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error) {
	logrus.WithContext(ctx).Debugf("[Codex] Using Responses image_generation tool for image edit (experimental), model: %s, mask: %t",
		req.Model, req.Mask != nil)

	responsesReq, err := buildImageEditResponsesRequest(&req)
	if err != nil {
		return nil, err
	}

	stream := c.OpenAIClient.ResponsesNewStreaming(ctx, responsesReq)
	return c.parseImageGenerationStream(ctx, stream)
}

// buildImageEditResponsesRequest translates ImageEditParams into a Responses
// request whose single user message carries the prompt plus every reference
// image, with the mask riding on the tool.
//
// It deliberately mirrors buildImageGenerationResponsesRequest (codex_client.go)
// on the request envelope — same store/instructions/parallel_tool_calls/include
// defaults — so the two image paths stay one shape, not two dialects.
func buildImageEditResponsesRequest(req *openai.ImageEditParams) (responses.ResponseNewParams, error) {
	readers := imageEditInputReaders(req)
	if len(readers) == 0 {
		return responses.ResponseNewParams{}, fmt.Errorf("image edit request has no input image")
	}
	if len(readers) > codexMaxReferenceImages {
		logrus.Debugf("[Codex] %d input images exceeds the Codex reference cap of %d; the backend may reject the request",
			len(readers), codexMaxReferenceImages)
	}

	contentItems := responses.ResponseInputMessageContentListParam{
		responses.ResponseInputContentParamOfInputText(req.Prompt),
	}
	for i, r := range readers {
		dataURL, err := readerToDataURL(r)
		if err != nil {
			return responses.ResponseNewParams{}, fmt.Errorf("failed to read input image %d: %w", i, err)
		}
		contentItems = append(contentItems, responses.ResponseInputContentUnionParam{
			OfInputImage: &responses.ResponseInputImageParam{
				// "high" rather than the default "auto": the reference image is
				// the thing being edited, not context to skim.
				Detail:   responses.ResponseInputImageDetailHigh,
				ImageURL: param.NewOpt(dataURL),
			},
		})
	}

	params := responses.ResponseNewParams{Model: req.Model}
	params.Store = param.NewOpt(false)
	params.Instructions = param.NewOpt(defaultInstructions)
	params.ParallelToolCalls = param.NewOpt(false)
	params.Include = []responses.ResponseIncludable{responses.ResponseIncludable(reasoningMarker)}
	params.Input = responses.ResponseNewParamsInputUnion{
		OfInputItemList: responses.ResponseInputParam{
			{
				OfMessage: &responses.EasyInputMessageParam{
					Type:    responses.EasyInputMessageTypeMessage,
					Role:    responses.EasyInputMessageRoleUser,
					Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: contentItems},
				},
			},
		},
	}

	outputFormat := "png"
	if req.OutputFormat != "" {
		outputFormat = string(req.OutputFormat)
	}

	toolParam := &responses.ToolImageGenerationParam{
		Type: "image_generation",
		// The whole point of this surface: say this is an edit of the images in
		// the message, not a fresh generation. "auto" would leave that to the
		// model, and a mask only means something for an edit.
		Action:       "edit",
		Size:         string(req.Size),
		Quality:      normalizeCodexImageQuality(string(req.Quality)),
		OutputFormat: outputFormat,
	}
	if req.InputFidelity != "" {
		toolParam.InputFidelity = string(req.InputFidelity)
	}
	if req.Background != "" {
		toolParam.Background = string(req.Background)
	}

	if req.Mask != nil {
		maskURL, err := readerToDataURL(req.Mask)
		if err != nil {
			return responses.ResponseNewParams{}, fmt.Errorf("failed to read mask: %w", err)
		}
		toolParam.InputImageMask = responses.ToolImageGenerationInputImageMaskParam{
			ImageURL: param.NewOpt(maskURL),
		}
	}

	params.Tools = []responses.ToolUnionParam{{OfImageGeneration: toolParam}}

	if req.N.Valid() && req.N.Value > 1 {
		logrus.Debugf("[Codex] Multiple images (N=%d) not supported on the Responses image path, using N=1", req.N.Value)
	}

	params.SetExtraFields(map[string]interface{}{"stream": true})

	return params, nil
}
