package executor

import (
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/thinking"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// decisionsAlt selects the Decisions API passthrough. It must stay aligned with
// the handler alt passed to ExecuteWithAuthManager.
const decisionsAlt = "decisions"

// prepareDecisionsPayload checks the Decisions request and rewrites only the
// model field. Input, questions, and unknown fields are left untouched, and
// prompt-cache or tool injection must not run on this payload.
func prepareDecisionsPayload(model string, payload []byte) ([]byte, error) {
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return nil, statusErr{code: http.StatusBadRequest, msg: "Invalid JSON body"}
	}
	parsed := gjson.ParseBytes(payload)
	input := parsed.Get("input")
	if !input.Exists() || input.Type == gjson.Null {
		return nil, statusErr{code: http.StatusBadRequest, msg: "Missing required parameter: 'input'"}
	}
	questions := parsed.Get("questions")
	if !questions.IsArray() || len(questions.Array()) == 0 {
		return nil, statusErr{code: http.StatusBadRequest, msg: "Missing required parameter: 'questions'"}
	}

	baseModel := strings.TrimSpace(thinking.ParseSuffix(model).ModelName)
	if baseModel == "" {
		baseModel = strings.TrimSpace(thinking.ParseSuffix(parsed.Get("model").String()).ModelName)
	}
	if baseModel == "" {
		return nil, statusErr{code: http.StatusBadRequest, msg: "Missing required parameter: 'model'"}
	}

	updated := helps.SetStringIfDifferent(payload, "model", baseModel)
	if parsed.Get("stream").Exists() {
		if deleted, errDelete := sjson.DeleteBytes(updated, "stream"); errDelete == nil {
			updated = deleted
		}
	}
	return updated, nil
}
