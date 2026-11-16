package compaction

import (
	"encoding/json"

	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"
)

// PreserveReplayFields restores fields unknown to the SDK on opaque history
// items. SDK request unions discard unknown fields on unmarshal; losing them
// breaks forward-compatible compact-to-continue and failover cloning. Keep the
// known fields typed so existing Codex ID sanitation continues to work.
func PreserveReplayFields(params *responses.ResponseNewParams, body []byte) error {
	items := gjson.GetBytes(body, "input").Array()
	for i := range params.Input.OfInputItemList {
		if i >= len(items) {
			break
		}
		item := &params.Input.OfInputItemList[i]
		var target interface {
			MarshalJSON() ([]byte, error)
			SetExtraFields(map[string]any)
		}
		switch {
		case item.OfCompaction != nil:
			target = item.OfCompaction
		case item.OfCompactionTrigger != nil:
			target = item.OfCompactionTrigger
		case item.OfReasoning != nil:
			target = item.OfReasoning
		default:
			continue
		}
		known, err := target.MarshalJSON()
		if err != nil {
			return err
		}
		knownFields := gjson.ParseBytes(known).Map()
		extra := map[string]any{}
		items[i].ForEach(func(key, value gjson.Result) bool {
			// Map lookup also preserves literal keys containing dots.
			if _, exists := knownFields[key.String()]; !exists {
				extra[key.String()] = json.RawMessage(value.Raw)
			}
			return true
		})
		if len(extra) > 0 {
			target.SetExtraFields(extra)
		}
	}
	return nil
}
