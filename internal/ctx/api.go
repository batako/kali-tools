package ctx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	apiFormatJSON  = "json"
	apiFormatShell = "shell"

	apiErrorInvalidRequestFormatVersion = "INVALID_REQUEST.FORMAT_VERSION"
	apiErrorNotFoundWorkspace           = "NOT_FOUND.WORKSPACE"
	apiErrorInternal                    = "INTERNAL_ERROR"
)

type APIResponse struct {
	Success       bool      `json:"success"`
	FormatVersion *string   `json:"format_version"`
	Data          any       `json:"data"`
	Error         *APIError `json:"error"`
}

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type outputOptions struct {
	Format        string
	FormatVersion string
}

var apiSupportedVersions = map[string][]string{
	"formats":    {"1.0"},
	"prompt":     {"1.0"},
	"credential": {"1.0"},
	"service":    {"1.0"},
}

var apiVersionPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

func parseOutputOptions(args []string, defaultFormat string) ([]string, outputOptions, error) {
	options := outputOptions{Format: defaultFormat}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return nil, outputOptions{}, errors.New("usage: --format <shell|json>")
			}
			i++
			options.Format = args[i]
		case "--format-version":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return nil, outputOptions{}, errors.New("usage: --format-version <version>")
			}
			i++
			options.FormatVersion = args[i]
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining, options, nil
}

func resolveAPIFormatVersion(endpoint, requested string) (string, error) {
	versions, ok := apiSupportedVersions[endpoint]
	if !ok || len(versions) == 0 {
		return "", fmt.Errorf("unknown JSON endpoint: %s", endpoint)
	}
	if requested != "" && !apiVersionPattern.MatchString(requested) {
		return "", apiFormatVersionError(endpoint, requested)
	}

	sorted := append([]string(nil), versions...)
	sort.Slice(sorted, func(i, j int) bool { return compareAPIVersion(sorted[i], sorted[j]) < 0 })

	if requested == "" {
		return sorted[len(sorted)-1], nil
	}
	if !strings.Contains(requested, ".") {
		prefix := requested + "."
		for i := len(sorted) - 1; i >= 0; i-- {
			if strings.HasPrefix(sorted[i], prefix) {
				return sorted[i], nil
			}
		}
		return "", apiFormatVersionError(endpoint, requested)
	}
	for _, version := range sorted {
		if version == requested {
			return version, nil
		}
	}
	return "", apiFormatVersionError(endpoint, requested)
}

func apiFormatVersionError(endpoint, requested string) error {
	versions := apiSupportedVersions[endpoint]
	return apiRequestError{
		Code:    apiErrorInvalidRequestFormatVersion,
		Message: "unsupported format version",
		Details: map[string]any{
			"endpoint":           endpoint,
			"requested_version":  requested,
			"supported_versions": append([]string(nil), versions...),
		},
		Invalid: true,
	}
}

func compareAPIVersion(a, b string) int {
	amajor, aminor := splitAPIVersion(a)
	bmajor, bminor := splitAPIVersion(b)
	if amajor != bmajor {
		return amajor - bmajor
	}
	return aminor - bminor
}

func splitAPIVersion(version string) (int, int) {
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}

type apiRequestError struct {
	Code    string
	Message string
	Details map[string]any
	Invalid bool
}

func (err apiRequestError) Error() string {
	return err.Message
}

func writeAPISuccess(stdout io.Writer, version string, data any) error {
	return writeAPIResponse(stdout, APIResponse{
		Success:       true,
		FormatVersion: &version,
		Data:          data,
		Error:         nil,
	})
}

func writeAPIError(stdout io.Writer, version *string, code, message string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	return writeAPIResponse(stdout, APIResponse{
		Success:       false,
		FormatVersion: version,
		Data:          nil,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func writeAPIInvalidRequest(stdout io.Writer, version *string, message string) error {
	if err := writeAPIError(stdout, version, "INVALID_REQUEST", message, map[string]any{}); err != nil {
		return err
	}
	return ExitCodeError{Code: 2}
}

func writeAPIResponse(stdout io.Writer, response APIResponse) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(response)
}

func runJSONEndpoint(stdout io.Writer, endpoint, requestedVersion string, build func(version string) (any, error)) error {
	version, err := resolveAPIFormatVersion(endpoint, requestedVersion)
	if err != nil {
		var requestErr apiRequestError
		if errors.As(err, &requestErr) {
			if writeErr := writeAPIError(stdout, nil, requestErr.Code, requestErr.Message, requestErr.Details); writeErr != nil {
				return writeErr
			}
			return ExitCodeError{Code: 2}
		}
		if writeErr := writeAPIError(stdout, nil, apiErrorInternal, "internal error", map[string]any{}); writeErr != nil {
			return writeErr
		}
		return ExitCodeError{Code: 1}
	}

	data, err := build(version)
	if err != nil {
		var requestErr apiRequestError
		if errors.As(err, &requestErr) {
			if writeErr := writeAPIError(stdout, &version, requestErr.Code, requestErr.Message, requestErr.Details); writeErr != nil {
				return writeErr
			}
			if requestErr.Invalid {
				return ExitCodeError{Code: 2}
			}
			return ExitCodeError{Code: 1}
		}
		code, message := apiErrorForError(err)
		if writeErr := writeAPIError(stdout, &version, code, message, map[string]any{}); writeErr != nil {
			return writeErr
		}
		if code == apiErrorInvalidRequestFormatVersion {
			return ExitCodeError{Code: 2}
		}
		return ExitCodeError{Code: 1}
	}
	if err := writeAPISuccess(stdout, version, data); err != nil {
		return err
	}
	return nil
}

func apiErrorForError(err error) (string, string) {
	if errors.Is(err, ErrWorkspaceNotFound) {
		return apiErrorNotFoundWorkspace, "no active workspace"
	}
	return apiErrorInternal, "internal error"
}

func apiStringOrNull(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func apiOptionalString(value string, valid bool) any {
	if !valid {
		return nil
	}
	return value
}

func formatsAPIData(version string) (any, error) {
	switch version {
	case "1.0":
		return formatsAPIDataV1_0(), nil
	default:
		return nil, fmt.Errorf("unsupported formats format version after resolution: %s", version)
	}
}

func formatsAPIDataV1_0() map[string]any {
	formats := make(map[string][]string, len(apiSupportedVersions))
	for endpoint, versions := range apiSupportedVersions {
		sorted := append([]string(nil), versions...)
		sort.Slice(sorted, func(i, j int) bool { return compareAPIVersion(sorted[i], sorted[j]) < 0 })
		formats[endpoint] = sorted
	}
	return map[string]any{"formats": formats}
}
