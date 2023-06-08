// MultipartEncoder implements the ability to serialize structures,
// map and single data into multipart/form-data content.
//
// The byte array should be implemented through the file
package mulipartencoder

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"reflect"
	"strings"
)

// toReaderFunc converts the value to the desired Reader
//
// Returns current Reader
type toReaderFunc func(reflect.Value) (io.Reader, error)

// encoderFunc provides a function to serialize the value in the desired format
type encoderFunc func(reflect.Value, string) error

func wrapError(err error) error {
	if err != nil {
		return fmt.Errorf("multipartencoder: %w", err)
	}
	return err
}

// Encoder is a layer on top of multipart.Writer for automatic data encoding
type Encoder struct {
	w *multipart.Writer
}

// Creates an Encoder instance
func NewEncoder(w *multipart.Writer) *Encoder {
	return &Encoder{w}
}

// Encoding struct or map fields to multipart/form-data fields
//
// # Does not accept any types other than map and struct
//
// Returns an error if unsuccessful
func (e *Encoder) Encode(v any) error {
	val := reflect.ValueOf(v)

	if !val.IsValid() {
		return wrapError(errors.New("val is not valid"))
	}

	kind := val.Kind()

	if kind == reflect.Pointer {
		val = reflect.Indirect(val)
		kind = val.Kind()
	}

	switch kind {
	case reflect.Struct:
		return wrapError(e.parseStruct(val))
	case reflect.Map:
		return wrapError(e.parseMap(val))
	}

	return wrapError(errors.New("only map or struct can be encoded"))
}

// Encoding the value in the multipart/form-data field
//
// struct and map will be written as a field with json
//
// Returns an error if unsuccessful
func (e *Encoder) EnecodeField(v any, fieldname string) error {
	val := reflect.ValueOf(v)

	if !val.IsValid() {
		return wrapError(errors.New("val is not valid"))
	}

	return wrapError(e.encodeField(val, fieldname))
}

// Parses and validates fields of struct
//
// Returns an error if unsuccessful
func (e *Encoder) parseStruct(val reflect.Value) error {
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldt := typ.Field(i)

		if !field.IsValid() {
			continue
		}

		if !fieldt.IsExported() {
			continue
		}

		tag := fieldt.Tag.Get("multipart")
		if tag == "" || tag == "-" {
			continue
		}

		if err := e.encodeField(field, tag); err != nil {
			return err
		}
	}

	return nil
}

// Parses fields and validates maps
func (e *Encoder) parseMap(val reflect.Value) error {
	for _, vkey := range val.MapKeys() {
		if vkey.Kind() != reflect.String {
			return errors.New("only a string must be a key")
		}
		key := vkey.Interface().(string)

		mval := val.MapIndex(vkey)

		if !mval.IsValid() {
			continue
		}
		if err := e.encodeField(val.MapIndex(vkey), key); err != nil {
			return err
		}
	}

	return nil
}

// Encoding the value in the multipart/form-data field
func (e *Encoder) encodeField(val reflect.Value, fieldname string) error {
	if k := val.Kind(); k == reflect.Pointer || k == reflect.Interface {
		if val.IsNil() {
			return nil
		}

		if k == reflect.Interface {
			val = val.Elem()
		}
	}

	if err := e.getEncoder(val)(val, fieldname); err != nil {
		return fmt.Errorf("field \"%s\": %w", fieldname, err)
	}
	return nil
}

// Selects the encoder for the value
//
// There is an exception in the form of a struct or map array.
// This variant is encoded as a single field instead of an array
//
// # Returns current encoderFunc
//
// # All encoding functions expect the correct value to be passed to them
//
// TODO: Maybe give the user a choice to encode such an array in a single field or in an array
func (e *Encoder) getEncoder(val reflect.Value) encoderFunc {
	kind := val.Kind()
	if kind == reflect.Pointer {
		kind = reflect.Indirect(val).Kind()
	}

	if kind == reflect.Array || kind == reflect.Slice {
		if k := deepTypeKind(val.Type()); k == reflect.Struct || k == reflect.Map {
			return e.encodeSingle
		}
		return e.encodeArray
	}

	if isFile(val) {
		return e.encodeFile
	}

	return e.encodeSingle
}

// Encoding a field containing an array in multipart/form-data array
func (e *Encoder) encodeArray(val reflect.Value, fieldname string) error {
	len := val.Len()
	if len <= 0 {
		return nil
	}

	if val.Kind() == reflect.Pointer {
		val = reflect.Indirect(val)
	}

	efunc := e.getEncoder(val.Index(0))

	for i := 0; i < len; i++ {
		err := efunc(val.Index(i), fmt.Sprintf("%s[]", fieldname))
		if err != nil {
			return err
		}
	}

	return nil
}

// Encoding a field containing an file in multipart/form-data file
func (e *Encoder) encodeFile(val reflect.Value, fieldname string) error {
	fs, err := fileStat(val)
	if err != nil {
		return fmt.Errorf("file in \"%s\" is not available", val.Type().Name())
	}

	filename := fs.Name()

	if fs.IsDir() {
		return fmt.Errorf("%s is dir", filename)
	}

	if fieldname == "" {
		fieldname = filename
	}

	fw, err := e.w.CreateFormFile(fieldname, filename)
	if err != nil {
		return err
	}

	_, err = io.Copy(fw, val.Interface().(io.Reader))

	return err
}

// Encodes primitive values, structures, map, and struct or map arrays into multipart/form-data field
func (e *Encoder) encodeSingle(val reflect.Value, fieldname string) error {
	if val.Kind() == reflect.Pointer {
		val = reflect.Indirect(val)
	}

	torfunc, err := getToReaderFunc(val.Type())
	if err != nil {
		return err
	}

	fw, err := e.w.CreateFormField(fieldname)
	if err != nil {
		return err
	}

	r, err := torfunc(val)
	if err != nil {
		return err
	}

	_, err = io.Copy(fw, r)

	return err
}

// Picks up the desired toReaderFunc function for the value.
//
// Cannot accept pointer or interface
func getToReaderFunc(t reflect.Type) (toReaderFunc, error) {
	kind := t.Kind()

	//TODO: add bytes
	switch t.Kind() {
	case reflect.Float32,
		reflect.Float64:
		return defaultToReader("%f"), nil
	case reflect.Int, reflect.Uint,
		reflect.Int8, reflect.Uint8,
		reflect.Int16, reflect.Uint16,
		reflect.Int32, reflect.Uint32,
		reflect.Int64, reflect.Uint64:
		return defaultToReader("%v"), nil
	case reflect.Bool:
		return defaultToReader("%t"), nil
	case reflect.String:
		return defaultToReader("%s"), nil
	case reflect.Map, reflect.Struct:
		return objectToReader, nil
	case reflect.Array, reflect.Slice:
		k := deepTypeKind(t)
		if k == reflect.Map || k == reflect.Struct {
			return objectToReader, nil
		}
		return nil, fmt.Errorf("array of %s is not supported type for reader", k.String())
	}

	return nil, fmt.Errorf("%s is not supported type for reader", kind.String())
}

// Outputs the reader for the values possible for formatting.
//
// TODO: Split into functions and replace fmt with strconv
func defaultToReader(format string) toReaderFunc {
	return func(v reflect.Value) (io.Reader, error) {
		return strings.NewReader(fmt.Sprintf(format, v.Interface())), nil
	}
}

// Used to convert struct, map, array of struct, array of map to json reader
func objectToReader(v reflect.Value) (io.Reader, error) {
	b := &bytes.Buffer{}
	data, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	b.Write(data)

	return b, nil
}

// Breaks through pointers and arrays to get kind
//
// Returns the final Kind
func deepTypeKind(val reflect.Type) reflect.Kind {
	k := val.Kind()

	if k == reflect.Array || k == reflect.Pointer || k == reflect.Slice {
		return deepTypeKind(val.Elem())
	}

	return val.Kind()
}

// Check if the type is a file
func isFile(val reflect.Value) bool {
	return val.MethodByName("Read").Kind() != reflect.Invalid && val.MethodByName("Stat").Kind() != reflect.Invalid
}

// Executes the fileStat function for the file value
func fileStat(val reflect.Value) (os.FileInfo, error) {
	r := val.MethodByName("Stat").Call([]reflect.Value{})
	if r[0].IsNil() {
		return nil, r[1].Interface().(error)
	}
	return r[0].Interface().(os.FileInfo), nil
}
