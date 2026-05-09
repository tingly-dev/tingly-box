package runtime

import coretool "github.com/tingly-dev/tingly-box/internal/tool"

type ContentType = coretool.ContentType

const (
	ContentTypeText  = coretool.ContentTypeText
	ContentTypeImage = coretool.ContentTypeImage
	ContentTypeBlob  = coretool.ContentTypeBlob
)

type ToolContent = coretool.ToolContent
type ToolResult = coretool.ToolResult

var TextToolResult = coretool.TextToolResult
var ErrorToolResult = coretool.ErrorToolResult
