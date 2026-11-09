package transform

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/tingly-dev/tingly-box/internal/protocol/msgparam"
)

func PrintBetaMessage(req *anthropic.BetaMessageNewParams) {
	for _, system := range req.System {
		fmt.Printf("%s\n", system.Text)
	}
	for _, message := range req.Messages {
		for _, block := range msgparam.BetaContent(message) {
			if block.OfText != nil {
				fmt.Printf("%s\n", block.OfText.Text)
			}
		}
	}
}

func PrintMessage(req *anthropic.MessageNewParams) {
	for _, system := range req.System {
		fmt.Printf("%s\n", system.Text)
	}
	for _, message := range req.Messages {
		for _, block := range msgparam.Content(message) {
			if block.OfText != nil {
				fmt.Printf("%s\n", block.OfText.Text)
			}
		}
	}
}
