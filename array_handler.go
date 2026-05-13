package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm/schema"
)

type PostgresArrayHandler struct{}

type arrayScanner struct {
	fieldType reflect.Type
	value     interface{}
}

func (s *arrayScanner) Scan(src interface{}) error {
	if src == nil {
		s.value = reflect.MakeSlice(s.fieldType.Elem(), 0, 0).Interface()
		return nil
	}

	switch v := src.(type) {
	case string:
		// Remove the curly braces
		str := strings.Trim(v, "{}")

		// Handle empty array
		if str == "" {
			s.value = reflect.MakeSlice(s.fieldType.Elem(), 0, 0).Interface()
			return nil
		}

		// Split the string into elements
		elements := strings.Split(str, ",")

		// Create a new slice with the correct type
		slice := reflect.MakeSlice(s.fieldType.Elem(), len(elements), len(elements))

		// Convert each element to the correct type
		for i, elem := range elements {
			elem = strings.Trim(elem, "\"") // Remove quotes if present
			switch s.fieldType.Elem().Elem().Kind() {
			case reflect.String:
				slice.Index(i).SetString(elem)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if val, err := strconv.ParseInt(elem, 10, 64); err == nil {
					slice.Index(i).SetInt(val)
				}
			case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if val, err := strconv.ParseUint(elem, 10, 64); err == nil {
					slice.Index(i).SetUint(val)
				}
			case reflect.Float32:
				if val, err := strconv.ParseFloat(elem, 32); err == nil {
					slice.Index(i).SetFloat(float64(val))
				}
			case reflect.Float64:
				if val, err := strconv.ParseFloat(elem, 64); err == nil {
					slice.Index(i).SetFloat(val)
				}
			case reflect.Bool:
				if val, err := strconv.ParseBool(elem); err == nil {
					slice.Index(i).SetBool(val)
				}
			}
		}
		s.value = slice.Interface()
		return nil
	}
	return fmt.Errorf("unsupported Scan, storing driver.Value type %T into type %s", src, s.fieldType)
}

func (s *arrayScanner) Value() (driver.Value, error) {
	if s.value == nil {
		return nil, nil
	}
	return s.value, nil
}

func (h *PostgresArrayHandler) HandleArray(field *schema.Field) error {
	oldValueOf := field.ValueOf
	field.ValueOf = func(ctx context.Context, v reflect.Value) (interface{}, bool) {
		value, zero := oldValueOf(ctx, v)
		if zero {
			return value, zero
		}

		return h.convertArrayToPostgres(value)
	}

	// Mark the field as implementing Scanner interface
	field.FieldType = reflect.PtrTo(field.FieldType)

	oldSet := field.Set
	field.Set = func(ctx context.Context, value reflect.Value, v interface{}) error {
		return h.handleArraySet(field, ctx, value, v, oldSet)
	}

	// Add Scanner implementation
	if _, ok := reflect.New(field.FieldType).Interface().(sql.Scanner); !ok {
		field.NewValuePool = &sync.Pool{
			New: func() interface{} {
				return &arrayScanner{
					fieldType: field.FieldType,
				}
			},
		}
	}

	return nil
}

func (h *PostgresArrayHandler) convertArrayToPostgres(value interface{}) (interface{}, bool) {
	switch slice := value.(type) {
	case []string:
		return "{" + strings.Join(slice, ",") + "}", false
	case []int:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatInt(int64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []int8:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatInt(int64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []int16:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatInt(int64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []int32:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatInt(int64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []int64:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatInt(v, 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []uint:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatUint(uint64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []uint16:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatUint(uint64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []uint32:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatUint(uint64(v), 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []uint64:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatUint(v, 10)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []float32:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []float64:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatFloat(v, 'f', -1, 64)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	case []bool:
		strs := make([]string, len(slice))
		for i, v := range slice {
			strs[i] = strconv.FormatBool(v)
		}
		return "{" + strings.Join(strs, ",") + "}", false
	}
	return value, false
}

func (h *PostgresArrayHandler) handleArraySet(field *schema.Field, ctx context.Context, value reflect.Value, v interface{}, oldSet func(context.Context, reflect.Value, interface{}) error) error {
	if v == nil {
		field.ReflectValueOf(ctx, value).Set(reflect.MakeSlice(field.FieldType.Elem(), 0, 0))
		return nil
	}

	switch data := v.(type) {
	case *arrayScanner:
		if data.value != nil {
			field.ReflectValueOf(ctx, value).Set(reflect.ValueOf(data.value))
		}
		return nil
	case string:
		// Remove the curly braces
		str := strings.Trim(data, "{}")

		// Handle empty array
		if str == "" {
			field.ReflectValueOf(ctx, value).Set(reflect.MakeSlice(field.FieldType.Elem(), 0, 0))
			return nil
		}

		// Split the string into elements
		elements := strings.Split(str, ",")

		// Create a new slice with the correct type
		slice := reflect.MakeSlice(field.FieldType.Elem(), len(elements), len(elements))

		// Convert each element to the correct type
		for i, elem := range elements {
			elem = strings.Trim(elem, "\"") // Remove quotes if present
			switch field.FieldType.Elem().Elem().Kind() {
			case reflect.String:
				slice.Index(i).SetString(elem)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if val, err := strconv.ParseInt(elem, 10, 64); err == nil {
					slice.Index(i).SetInt(val)
				}
			case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				if val, err := strconv.ParseUint(elem, 10, 64); err == nil {
					slice.Index(i).SetUint(val)
				}
			case reflect.Float32:
				if val, err := strconv.ParseFloat(elem, 32); err == nil {
					slice.Index(i).SetFloat(val)
				}
			case reflect.Float64:
				if val, err := strconv.ParseFloat(elem, 64); err == nil {
					slice.Index(i).SetFloat(val)
				}
			case reflect.Bool:
				if val, err := strconv.ParseBool(elem); err == nil {
					slice.Index(i).SetBool(val)
				}
			}
		}
		field.ReflectValueOf(ctx, value).Set(slice)
		return nil
	default:
		return oldSet(ctx, value, v)
	}
}
