package validator

import (
	"html"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin/binding"
	validatorpkg "github.com/go-playground/validator/v10"
	"github.com/stellar/go/keypair"
	"github.com/yourusername/kor-assetforge/utils"
)

var validate *validatorpkg.Validate

func Init() error {
	if v, ok := binding.Validator.Engine().(*validatorpkg.Validate); ok {
		validate = v
	} else {
		validate = validatorpkg.New()
	}

	if err := validate.RegisterValidation("stellar_address", stellarAddress); err != nil {
		return err
	}
	if err := validate.RegisterValidation("no_html", noHTML); err != nil {
		return err
	}
	if err := validate.RegisterValidation("asset_symbol", assetSymbol); err != nil {
		return err
	}
	if err := validate.RegisterValidation("asset_type", assetType); err != nil {
		return err
	}
	if err := validate.RegisterValidation("metadata_uri", metadataURI); err != nil {
		return err
	}
	if err := validate.RegisterValidation("metadata_hash", metadataHash); err != nil {
		return err
	}

	return nil
}

func ValidateStruct(s interface{}) error {
	if validate == nil {
		if err := Init(); err != nil {
			return err
		}
	}
	return validate.Struct(s)
}

func SanitizeString(value string) string {
	sanitized := strings.TrimSpace(value)
	sanitized = html.EscapeString(sanitized)
	sanitized = strings.Join(strings.Fields(sanitized), " ")
	return sanitized
}

func SanitizeStruct(data interface{}) {
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	v = v.Elem()
	sanitizeValue(v)
}

func sanitizeValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			sanitizeValue(v.Elem())
		}
	case reflect.Interface:
		if !v.IsNil() {
			sanitizeValue(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if field.CanSet() {
				sanitizeValue(field)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			sanitizeValue(v.Index(i))
		}
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return
		}
		for _, key := range v.MapKeys() {
			value := v.MapIndex(key)
			if !value.IsValid() {
				continue
			}
			if value.Kind() == reflect.String {
				sanitized := SanitizeString(value.String())
				v.SetMapIndex(key, reflect.ValueOf(sanitized))
			}
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(utils.SanitizeUserContent(strings.TrimSpace(v.String())))
		}
	}
}

func stellarAddress(fl validatorpkg.FieldLevel) bool {
	addr, ok := fl.Field().Interface().(string)
	if !ok || strings.TrimSpace(addr) == "" {
		return false
	}
	_, err := keypair.ParseAddress(strings.TrimSpace(addr))
	return err == nil
}

func noHTML(fl validatorpkg.FieldLevel) bool {
	value, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	return !strings.ContainsAny(trimmed, "<>")
}

func assetSymbol(fl validatorpkg.FieldLevel) bool {
	value, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}
	symbol := strings.TrimSpace(value)
	if len(symbol) < 3 || len(symbol) > 10 {
		return false
	}
	matched, _ := regexp.MatchString("^[A-Z0-9]+$", symbol)
	return matched
}

func assetType(fl validatorpkg.FieldLevel) bool {
	value, ok := fl.Field().Interface().(string)
	if !ok {
		return false
	}
	assetType := strings.TrimSpace(value)
	if len(assetType) == 0 || len(assetType) > 50 {
		return false
	}
	return !strings.ContainsAny(assetType, "<>")
}

func metadataURI(fl validatorpkg.FieldLevel) bool {
	value, ok := fl.Field().Interface().(string)
	if !ok || value == "" {
		return true
	}
	return strings.HasPrefix(value, "ipfs://") || strings.HasPrefix(value, "https://")
}

func metadataHash(fl validatorpkg.FieldLevel) bool {
	value, ok := fl.Field().Interface().(string)
	if !ok || value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
