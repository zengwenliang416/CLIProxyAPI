package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/tidwall/gjson"
)

var errImageStreamMissingFinal = errors.New("upstream image stream closed before final image")

type imageSSECompletionState struct {
	frames    sseJSONValidationState
	completed bool
}

func (s *imageSSECompletionState) AddChunk(chunk []byte) error {
	frames, errValidate := s.frames.AddChunk(chunk)
	if errValidate != nil {
		return errValidate
	}
	s.observeFrames(frames)
	return nil
}

func (s *imageSSECompletionState) Finish() error {
	if errValidate := s.frames.Finish(); errValidate != nil {
		return errValidate
	}
	if !s.completed {
		return errImageStreamMissingFinal
	}
	return nil
}

func (s *imageSSECompletionState) observeFrames(frames []byte) {
	for len(frames) > 0 {
		frameEnd := bytes.Index(frames, []byte("\n\n"))
		if frameEnd < 0 {
			s.observeFrame(frames)
			return
		}
		frameEnd += 2
		s.observeFrame(frames[:frameEnd])
		frames = frames[frameEnd:]
	}
}

func (s *imageSSECompletionState) observeFrame(frame []byte) {
	payload, found := sseJSONValidationDataPayload(frame)
	payload = bytes.TrimSpace(payload)
	if !found || len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
		return
	}

	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType == "" {
		eventType = sseEventName(frame)
	}
	if !isCompletedImageStreamEvent(eventType) {
		return
	}

	if strings.TrimSpace(gjson.GetBytes(payload, "b64_json").String()) != "" ||
		strings.TrimSpace(gjson.GetBytes(payload, "url").String()) != "" {
		s.completed = true
	}
}

func isCompletedImageStreamEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "image_generation.completed", "image_edit.completed":
		return true
	default:
		return false
	}
}

func sseEventName(frame []byte) string {
	for _, line := range bytes.Split(frame, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("event:")) {
			return strings.TrimSpace(string(line[len("event:"):]))
		}
	}
	return ""
}

func imageStreamErrorMessage(err error) *interfaces.ErrorMessage {
	var authErr *coreauth.Error
	if errors.As(err, &authErr) && authErr != nil && strings.TrimSpace(authErr.Code) == "empty_stream" {
		return &interfaces.ErrorMessage{StatusCode: http.StatusBadGateway, Error: errImageStreamMissingFinal}
	}
	return executionErrorMessage(err)
}
