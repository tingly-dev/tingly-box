package protocol

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_AnthropicUnMarshal(t *testing.T) {
	for _, d := range testdata {
		r := AnthropicBetaMessagesRequest{}

		err := r.UnmarshalJSON([]byte(d))
		if err != nil {
			t.Error(err)
		}
		last := r.Messages[len(r.Messages)-1]
		content := ""
		for _, block := range last.Content {
			for _, tb := range block.OfToolResult.Content {
				content = tb.OfText.Text
			}
		}
		bs, err := r.MarshalJSON()
		t.Log(string(bs))
		assert.True(t, strings.Contains(content, "上海"))
	}
}

var testdata []string

func init() {
	testdata = []string{`
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 1024,
  "tools": [
    {
      "name": "get_weather",
      "description": "获取指定城市的实时天气信息",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string",
            "description": "城市名称，例如：北京、上海"
          },
          "unit": {
            "type": "string",
            "enum": ["celsius", "fahrenheit"],
            "description": "温度单位"
          }
        },
        "required": ["city"]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "上海今天天气怎么样？"
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "text",
          "text": "我来帮你查询上海的天气。"
        },
        {
          "type": "tool_use",
          "id": "toolu_01XQmkbAr2PVKdKPkrCmNsqG",
          "name": "get_weather",
          "input": {
            "city": "上海",
            "unit": "celsius"
          }
        }
      ]
    },
    {
      "role": "user",
      "content": [
        {
          "type": "tool_result",
          "tool_use_id": "toolu_01XQmkbAr2PVKdKPkrCmNsqG",
          "content": [
            {
              "type": "text",
              "text": "{\"city\": \"上海\", \"temperature\": 22, \"unit\": \"celsius\", \"condition\": \"多云\", \"humidity\": 68, \"wind\": \"东南风 3级\"}"
            }
          ]
        }
      ]
    }
  ]
}
`,
		`
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 1024,
  "tools": [
    {
      "name": "get_weather",
      "description": "获取指定城市的实时天气信息",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {
            "type": "string",
            "description": "城市名称，例如：北京、上海"
          },
          "unit": {
            "type": "string",
            "enum": ["celsius", "fahrenheit"],
            "description": "温度单位"
          }
        },
        "required": ["city"]
      }
    }
  ],
  "messages": [
    {
      "role": "user",
      "content": "上海今天天气怎么样？"
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "text",
          "text": "我来帮你查询上海的天气。"
        },
        {
          "type": "tool_use",
          "id": "toolu_01XQmkbAr2PVKdKPkrCmNsqG",
          "name": "get_weather",
          "input": {
            "city": "上海",
            "unit": "celsius"
          }
        }
      ]
    },
    {
      "role": "user",
      "content": [
        {
          "type": "tool_result",
          "tool_use_id": "toolu_01XQmkbAr2PVKdKPkrCmNsqG",
          "content": "{\"city\": \"上海\", \"temperature\": 22, \"unit\": \"celsius\", \"condition\": \"多云\", \"humidity\": 68, \"wind\": \"东南风 3级\"}"
        }
      ]
    }
  ]
}
`,
	}
}
