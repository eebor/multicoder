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

type toReaderFunc func(reflect.Value) (io.Reader, error)
type encoderFunc func(reflect.Value, string) error

func wrapError(err error) error {
	if err != nil {
		return fmt.Errorf("multipartencoder: %w", err)
	}
	return err
}

type Encoder struct {
	w *multipart.Writer
}

func NewEncoder(w *multipart.Writer) *Encoder {
	return &Encoder{w}
}

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

func (e *Encoder) EnecodeField(v any, fieldname string) error {
	val := reflect.ValueOf(v)

	if !val.IsValid() {
		return wrapError(errors.New("val is not valid"))
	}

	return wrapError(e.encodeField(val, fieldname))
}

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

func defaultToReader(format string) toReaderFunc {
	return func(v reflect.Value) (io.Reader, error) {
		return strings.NewReader(fmt.Sprintf(format, v.Interface())), nil
	}
}

func objectToReader(v reflect.Value) (io.Reader, error) {
	b := &bytes.Buffer{}
	data, err := json.Marshal(v.Interface())
	if err != nil {
		return nil, err
	}
	b.Write(data)

	return b, nil
}

func deepTypeKind(arr reflect.Type) reflect.Kind {
	k := arr.Kind()

	if k == reflect.Array || k == reflect.Pointer || k == reflect.Slice {
		return deepTypeKind(arr.Elem())
	}

	return arr.Kind()
}

func isFile(val reflect.Value) bool {
	return val.MethodByName("Read").Kind() != reflect.Invalid && val.MethodByName("Stat").Kind() != reflect.Invalid
}

func fileStat(val reflect.Value) (os.FileInfo, error) {
	r := val.MethodByName("Stat").Call([]reflect.Value{})
	if r[0].IsNil() {
		return nil, r[1].Interface().(error)
	}
	return r[0].Interface().(os.FileInfo), nil
}
