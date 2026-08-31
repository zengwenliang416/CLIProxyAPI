package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

const DefaultMaxDecodedRequestBodyBytes int64 = 80 << 20

const requestAdmissionReleaseContextKey = "cliproxy.request-admission-release"

// RequestBodyTooLargeError reports that an encoded or decoded request body
// exceeded the configured limit.
type RequestBodyTooLargeError struct {
	Limit int64
}

func (e *RequestBodyTooLargeError) Error() string {
	return fmt.Sprintf("request body exceeds the %d byte limit", e.Limit)
}

// IsRequestBodyTooLarge reports whether err represents a request body limit.
func IsRequestBodyTooLarge(err error) bool {
	var tooLarge *RequestBodyTooLargeError
	var maxBytes *http.MaxBytesError
	return errors.As(err, &tooLarge) || errors.As(err, &maxBytes)
}

// WriteRequestBodyError writes the protocol-neutral invalid request response.
func WriteRequestBodyError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	errType := "invalid_request_error"
	errCode := ""
	if IsRequestCapacityUnavailable(err) {
		status = http.StatusServiceUnavailable
		errType = "server_error"
		errCode = "request_capacity_unavailable"
	} else if IsRequestBodyTooLarge(err) {
		status = http.StatusRequestEntityTooLarge
	}
	messagePrefix := "Invalid request"
	if IsRequestCapacityUnavailable(err) {
		messagePrefix = "Request temporarily unavailable"
	}
	c.JSON(status, ErrorResponse{
		Error: ErrorDetail{
			Message: fmt.Sprintf("%s: %v", messagePrefix, err),
			Type:    errType,
			Code:    errCode,
		},
	})
}

// ReadRequestBody reads the incoming request body and decodes supported
// Content-Encoding values before handlers inspect JSON fields.
func ReadRequestBody(c *gin.Context) ([]byte, error) {
	return readRequestBody(c, DefaultMaxDecodedRequestBodyBytes, DefaultLargeRequestThresholdBytes, nil)
}

// ReadRequestBody applies the handler's configured decoded body limit.
func (h *BaseAPIHandler) ReadRequestBody(c *gin.Context) ([]byte, error) {
	limit := h.MaxRequestBodyBytes()
	threshold := h.LargeRequestThresholdBytes()
	var admission *RequestAdmissionController
	if h != nil {
		if h.requestAdmission == nil {
			h.requestAdmission = NewRequestAdmissionController(h.Cfg)
		}
		admission = h.requestAdmission
	}
	return readRequestBody(c, limit, threshold, admission)
}

// ReadAdmittedPayload spools a non-HTTP payload, acquires weighted capacity
// from its actual size, and materializes it only after admission.
func (h *BaseAPIHandler) ReadAdmittedPayload(ctx context.Context, reader io.Reader) ([]byte, func(), error) {
	limit := h.MaxRequestBodyBytes()
	spool, err := spoolRequestBody(nil, reader, limit, h.LargeRequestThresholdBytes())
	if err != nil {
		return nil, nil, err
	}
	defer spool.Close()

	release, err := h.AcquireRequestCapacity(ctx, spool.Size())
	if err != nil {
		return nil, nil, err
	}
	payload, err := spool.Bytes()
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("read spooled payload: %w", err)
	}
	return payload, release, nil
}

// ReadRequestBodyWithLimit reads and decodes a request body while bounding both
// the encoded and decoded representations.
func ReadRequestBodyWithLimit(c *gin.Context, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = DefaultMaxDecodedRequestBodyBytes
	}
	return readRequestBody(c, limit, min(limit, DefaultLargeRequestThresholdBytes), nil)
}

func readRequestBody(c *gin.Context, limit, memoryThreshold int64, admission *RequestAdmissionController) ([]byte, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("request is unavailable")
	}
	if limit <= 0 {
		limit = DefaultMaxDecodedRequestBodyBytes
	}
	if memoryThreshold <= 0 || memoryThreshold > limit {
		memoryThreshold = min(limit, DefaultLargeRequestThresholdBytes)
	}

	encoded, err := spoolRequestBody(c.Writer, c.Request.Body, limit, memoryThreshold)
	if err != nil {
		if IsRequestBodyTooLarge(err) {
			return nil, &RequestBodyTooLargeError{Limit: limit}
		}
		return nil, err
	}
	defer encoded.Close()

	encoding := strings.TrimSpace(c.Request.Header.Get("Content-Encoding"))
	decoded := encoded
	if encoding == "" || strings.EqualFold(encoding, "identity") {
		decoded = encoded
	} else {
		decoded, err = decodeRequestBodySpool(encoded, encoding, limit, memoryThreshold)
		if err != nil {
			if IsRequestBodyTooLarge(err) {
				return nil, err
			}
			raw, errRaw := encoded.Bytes()
			if errRaw == nil && json.Valid(raw) {
				decoded = encoded
			} else {
				return nil, err
			}
		}
		if decoded != encoded {
			defer decoded.Close()
		}
	}

	release := func() {}
	if admission != nil {
		release, err = admission.Acquire(c.Request.Context(), decoded.Size())
		if err != nil {
			return nil, err
		}
	}
	body, err := decoded.Bytes()
	if err != nil {
		release()
		return nil, fmt.Errorf("read spooled request body: %w", err)
	}
	bindRequestAdmission(c, release)
	return body, nil
}

func decodeRequestBodySpool(raw *requestBodySpool, encoding string, limit, memoryThreshold int64) (*requestBodySpool, error) {
	parts := strings.Split(encoding, ",")
	body := raw
	owned := false
	for i := len(parts) - 1; i >= 0; i-- {
		enc := strings.ToLower(strings.TrimSpace(parts[i]))
		switch enc {
		case "", "identity":
			continue
		case "zstd":
			decoded, err := decodeZstdRequestBodySpool(body, limit, memoryThreshold)
			if err != nil {
				if owned {
					_ = body.Close()
				}
				return nil, err
			}
			if owned {
				_ = body.Close()
			}
			body = decoded
			owned = true
		default:
			if owned {
				_ = body.Close()
			}
			return nil, fmt.Errorf("unsupported request content encoding: %s", enc)
		}
	}
	return body, nil
}

func decodeZstdRequestBodySpool(raw *requestBodySpool, limit, memoryThreshold int64) (*requestBodySpool, error) {
	reader, err := raw.Reader()
	if err != nil {
		return nil, fmt.Errorf("open zstd request body: %w", err)
	}
	maxWindow := uint64(max(limit, 1024))
	decoder, err := zstd.NewReader(
		reader,
		zstd.WithDecoderMaxMemory(uint64(limit)),
		zstd.WithDecoderMaxWindow(maxWindow),
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderLowmem(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd request decoder: %w", err)
	}
	defer decoder.Close()

	decoded, err := spoolRequestBody(nil, decoder, limit, memoryThreshold)
	if err != nil {
		if IsRequestBodyTooLarge(err) || strings.Contains(strings.ToLower(err.Error()), "decompressed size exceeds configured limit") {
			return nil, &RequestBodyTooLargeError{Limit: limit}
		}
		return nil, fmt.Errorf("failed to decode zstd request body: %w", err)
	}
	return decoded, nil
}

type requestBodySpool struct {
	memoryLimit int64
	size        int64
	memory      bytes.Buffer
	file        *os.File
}

func spoolRequestBody(writer http.ResponseWriter, reader io.Reader, limit, memoryLimit int64) (*requestBodySpool, error) {
	if reader == nil {
		reader = http.NoBody
	}
	if writer != nil {
		reader = http.MaxBytesReader(writer, io.NopCloser(reader), limit)
	}
	spool := &requestBodySpool{memoryLimit: memoryLimit}
	_, err := io.Copy(spool, io.LimitReader(reader, limit+1))
	if err != nil {
		_ = spool.Close()
		if IsRequestBodyTooLarge(err) {
			return nil, &RequestBodyTooLargeError{Limit: limit}
		}
		return nil, err
	}
	if spool.size > limit {
		_ = spool.Close()
		return nil, &RequestBodyTooLargeError{Limit: limit}
	}
	return spool, nil
}

func (s *requestBodySpool) Write(p []byte) (int, error) {
	if s.file == nil && s.size+int64(len(p)) <= s.memoryLimit {
		n, err := s.memory.Write(p)
		s.size += int64(n)
		return n, err
	}
	if s.file == nil {
		file, err := os.CreateTemp("", "cliproxy-request-*")
		if err != nil {
			return 0, err
		}
		s.file = file
		if _, err = s.file.Write(s.memory.Bytes()); err != nil {
			_ = s.Close()
			return 0, err
		}
		s.memory.Reset()
	}
	n, err := s.file.Write(p)
	s.size += int64(n)
	return n, err
}

func (s *requestBodySpool) Size() int64 {
	if s == nil {
		return 0
	}
	return s.size
}

func (s *requestBodySpool) Reader() (io.Reader, error) {
	if s.file == nil {
		return bytes.NewReader(s.memory.Bytes()), nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return s.file, nil
}

func (s *requestBodySpool) Bytes() ([]byte, error) {
	reader, err := s.Reader()
	if err != nil {
		return nil, err
	}
	body := make([]byte, s.size)
	_, err = io.ReadFull(reader, body)
	return body, err
}

func (s *requestBodySpool) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	name := s.file.Name()
	errClose := s.file.Close()
	errRemove := os.Remove(name)
	s.file = nil
	if errClose != nil {
		return errClose
	}
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return errRemove
	}
	return nil
}

func bindRequestAdmission(c *gin.Context, release func()) {
	if c == nil || release == nil {
		return
	}
	ReleaseRequestBody(c)

	var once sync.Once
	var stop func() bool
	wrapped := func() {
		once.Do(func() {
			if stop != nil {
				stop()
			}
			release()
		})
	}
	if c.Request != nil {
		stop = context.AfterFunc(c.Request.Context(), wrapped)
	}
	c.Set(requestAdmissionReleaseContextKey, wrapped)
}

// ReleaseRequestBody releases any weighted admission held by the current
// request. It is safe to call more than once.
func ReleaseRequestBody(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get(requestAdmissionReleaseContextKey)
	if !ok {
		return
	}
	c.Set(requestAdmissionReleaseContextKey, nil)
	if release, okRelease := value.(func()); okRelease && release != nil {
		release()
	}
}

// ParseRequestForm applies the same absolute and weighted admission boundaries
// to multipart and URL-encoded forms before handlers inspect form values.
func (h *BaseAPIHandler) ParseRequestForm(c *gin.Context) error {
	if c == nil || c.Request == nil {
		return fmt.Errorf("request is unavailable")
	}
	limit := h.MaxRequestBodyBytes()
	if c.Request.ContentLength > limit {
		return &RequestBodyTooLargeError{Limit: limit}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	contentType := strings.ToLower(strings.TrimSpace(c.Request.Header.Get("Content-Type")))
	var err error
	if strings.HasPrefix(contentType, "multipart/form-data") {
		err = c.Request.ParseMultipartForm(h.LargeRequestThresholdBytes())
	} else {
		err = c.Request.ParseForm()
	}
	if err != nil {
		CleanupRequestForm(c)
		if IsRequestBodyTooLarge(err) {
			return &RequestBodyTooLargeError{Limit: limit}
		}
		return err
	}

	size := c.Request.ContentLength
	if size < 0 {
		size = 0
	}
	parsedSize := int64(0)
	for key, values := range c.Request.Form {
		parsedSize += int64(len(key))
		for _, value := range values {
			parsedSize += int64(len(value))
		}
	}
	if form := c.Request.MultipartForm; form != nil {
		for key, files := range form.File {
			parsedSize += int64(len(key))
			for _, file := range files {
				if file != nil {
					parsedSize += file.Size
				}
			}
		}
	}
	size = max(size, parsedSize)
	release, err := h.AcquireRequestCapacity(c.Request.Context(), size)
	if err != nil {
		CleanupRequestForm(c)
		return err
	}
	bindRequestAdmission(c, release)
	return nil
}

// CleanupRequestForm removes disk-backed multipart files created by net/http.
func CleanupRequestForm(c *gin.Context) {
	if c == nil || c.Request == nil || c.Request.MultipartForm == nil {
		return
	}
	_ = c.Request.MultipartForm.RemoveAll()
}
