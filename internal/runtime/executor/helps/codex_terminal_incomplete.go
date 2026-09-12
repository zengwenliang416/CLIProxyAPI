package helps

import (
	"strings"

	"github.com/tidwall/gjson"
)

const CodexEmptyIncompleteStreamMessage = "stream error: upstream terminated with incomplete empty response (0 tokens)"

func HasMeaningfulCodexOutputDelta(eventData []byte) bool {
	eventType := gjson.GetBytes(eventData, "type").String()
	switch eventType {
	case "response.output_text.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta":
		delta := gjson.GetBytes(eventData, "delta")
		return delta.Exists() && len(strings.TrimSpace(delta.String())) > 0
	}
	return false
}

func IsCodexTerminalEmptyIncomplete(eventData []byte, outputItemsCount int, sawOutputDelta bool) bool {
	if gjson.GetBytes(eventData, "type").String() != "response.incomplete" || sawOutputDelta || outputItemsCount > 0 {
		return false
	}
	output := gjson.GetBytes(eventData, "response.output")
	if output.Exists() && output.IsArray() && len(output.Array()) > 0 {
		return false
	}
	outputTokens := gjson.GetBytes(eventData, "response.usage.output_tokens")
	return outputTokens.Exists() && outputTokens.Type == gjson.Number && outputTokens.Num == 0 && strings.TrimSpace(outputTokens.Raw) == "0"
}
