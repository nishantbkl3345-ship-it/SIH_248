package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"fogline/api/internal/apperr"
)

var registerJSONNames sync.Once

// useJSONFieldNames makes validation errors refer to fields by their JSON
// name, which is what the client sent.
func useJSONFieldNames() {
	registerJSONNames.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				return f.Name
			}
			return name
		})
	})
}

// bindJSON decodes and validates the request body into dst. On failure it
// records the error for errorHandler and returns false.
func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		_ = c.Error(bindError(err))
		return false
	}
	return true
}

func bindError(err error) *apperr.Error {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		details := make(map[string]string, len(verrs))
		for _, fe := range verrs {
			details[fieldPath(fe)] = fieldMessage(fe)
		}
		return apperr.Validation(details)
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return apperr.Validation(map[string]string{typeErr.Field: "has the wrong type"})
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return apperr.New(http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body is too large.")
	}
	if errors.Is(err, io.EOF) {
		return apperr.BadRequest("INVALID_JSON", "Request body is required.")
	}
	return apperr.BadRequest("INVALID_JSON", "Request body is not valid JSON.")
}

// embeddedSegments are embedded request structs. The validator names them in
// a field's namespace, but they do not exist in the JSON the client sent.
var embeddedSegments = strings.NewReplacer("scenarioContent.", "")

// fieldPath is the field's location in the request body, e.g.
// "phases[0].reports[2].title": the validator's namespace without the root
// struct name.
func fieldPath(fe validator.FieldError) string {
	_, path, ok := strings.Cut(fe.Namespace(), ".")
	if !ok {
		return fe.Field()
	}
	return embeddedSegments.Replace(path)
}

func fieldMessage(fe validator.FieldError) string {
	unit := " characters"
	switch fe.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
		unit = ""
	case reflect.Slice, reflect.Array, reflect.Map:
		unit = " items"
	}
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "uuid":
		return "must be a valid id"
	case "gt":
		return "must be greater than " + fe.Param()
	case "min":
		return fmt.Sprintf("must be at least %s%s", fe.Param(), unit)
	case "max":
		return fmt.Sprintf("must be at most %s%s", fe.Param(), unit)
	case "oneof":
		return "must be one of: " + strings.Join(strings.Fields(fe.Param()), ", ")
	default:
		return "is invalid"
	}
}
