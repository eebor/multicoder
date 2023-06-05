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

type toReaderFunc func(reflect.Value) io.Reader
type encoderFunc func(reflect.Value, string) error

func wrapError(err error) error {
	return fmt.Errorf("multipartencoder: %s", err.Error())
}

type Encoder struct {
	w *multipart.Writer
}

func NewEncoder(w *multipart.Writer) *Encoder {
	return &Encoder{w}
}

func (e *Encoder) Encode(v any) error {
	val := reflect.ValueOf(v)
	kind := val.Kind()

	switch kind {
	case reflect.Struct:
		return e.parseStruct(val)
	case reflect.Map:
		return e.parseMap(val)
	}

	return errors.New("only map or struct can be encoded")
}

func (e *Encoder) parseStruct(val reflect.Value) error {
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldt := typ.Field(i)

		if !fieldt.IsExported() {
			continue
		}

		tag := fieldt.Tag.Get("multipart")
		if tag == "" {
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
			return wrapError(errors.New("only a string must be a key"))
		}
		key := vkey.Interface().(string)

		if err := e.encodeField(val.MapIndex(vkey), key); err != nil {
			return err
		}
	}

	return nil
}

func (e *Encoder) encodeField(val reflect.Value, fieldname string) error {
	if k := val.Kind(); k == reflect.Interface || k == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		if k == reflect.Interface {
			val = val.Elem()
		}
	}

	return e.getEncoder(val)(val, fieldname)
}

func (e *Encoder) getEncoder(val reflect.Value) encoderFunc {
	kind := val.Kind()
	if kind == reflect.Pointer {
		e.getEncoder(val.Elem())
	}

	if kind == reflect.Array || kind == reflect.Slice {
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

	efunc := e.getEncoder(val.Index(0))

	for i := 0; i < len; i++ {
		err := efunc(val.Index(i), fmt.Sprintf("%s[%v]", fieldname, i))
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Encoder) encodeFile(val reflect.Value, fieldname string) error {
	fs, err := fileStat(val)
	if err != nil {
		return wrapError(fmt.Errorf("file in \"%s\" is not available", val.Type().Name()))
	}

	filename := fs.Name()

	if fs.IsDir() {
		return wrapError(fmt.Errorf("%s is dir", filename))
	}

	if fieldname == "" {
		fieldname = filename
	}

	fw, err := e.w.CreateFormFile(fieldname, filename)
	if err != nil {
		return wrapError(err)
	}

	_, err = io.Copy(fw, val.Interface().(io.Reader))

	return err
}

func (e *Encoder) encodeSingle(val reflect.Value, fieldname string) error {
	torfunc, err := getToReaderFunc(val.Type())
	if err != nil {
		return err
	}

	fw, err := e.w.CreateFormField(fieldname)
	if err != nil {
		return err
	}

	_, err = io.Copy(fw, torfunc(val))

	return err
}

func getToReaderFunc(t reflect.Type) (toReaderFunc, error) {
	kind := t.Kind()

	if kind == reflect.Pointer {
		return getToReaderFunc(t.Elem())
	}

	switch kind {
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
		return defaultToReader("%b"), nil
	case reflect.String:
		return defaultToReader("%s"), nil
	case reflect.Map, reflect.Struct:
		return objectToReader, nil
	}

	return nil, wrapError(fmt.Errorf("%s is not supported type", kind.String()))
}

func defaultToReader(format string) toReaderFunc {
	return func(v reflect.Value) io.Reader {
		return strings.NewReader(fmt.Sprintf(format, v.Interface()))
	}
}

func objectToReader(v reflect.Value) io.Reader {
	b := &bytes.Buffer{}
	json.NewEncoder(b).Encode(v.Interface())
	return b
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
