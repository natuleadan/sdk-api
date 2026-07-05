package middleware

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

var validate = validator.New()
var validationModels = make(map[string]reflect.Type)

func RegisterValidation(name string, input any) {
	t := reflect.TypeOf(input)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	validationModels[name] = t
}

func ValidateInput(modelName string) fiber.Handler {
	return func(c fiber.Ctx) error {
		inputType, ok := validationModels[modelName]
		if !ok {
			return c.Status(500).JSON(fiber.Map{
				"code":    500,
				"message": fmt.Sprintf("validation model %q not registered", modelName),
			})
		}

		input := reflect.New(inputType).Interface()
		if err := c.Bind().Body(input); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"code":    400,
				"message": "invalid request body",
			})
		}

		if err := validate.Struct(input); err != nil {
			if errs, ok := err.(validator.ValidationErrors); ok {
				fields := make(map[string]string)
				for _, e := range errs {
					fields[e.Field()] = validationError(e)
				}
				return c.Status(422).JSON(fiber.Map{
					"code":    422,
					"message": "validation failed",
					"fields":  fields,
				})
			}
			return c.Status(422).JSON(fiber.Map{
				"code":    422,
				"message": err.Error(),
			})
		}

		c.Locals("validated_input", input)
		return c.Next()
	}
}

func validationError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", e.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s", e.Field(), e.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", e.Field(), e.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", e.Field(), e.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", e.Field(), e.Param())
	case "email":
		return fmt.Sprintf("%s must be a valid email", e.Field())
	case "url":
		return fmt.Sprintf("%s must be a valid URL", e.Field())
	case "oneof":
		return fmt.Sprintf("%s must be one of %s", e.Field(), strings.ReplaceAll(e.Param(), " ", ", "))
	case "alphanum":
		return fmt.Sprintf("%s must be alphanumeric", e.Field())
	default:
		return fmt.Sprintf("%s failed %s validation", e.Field(), e.Tag())
	}
}
