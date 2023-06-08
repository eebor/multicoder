package multicoder

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"os"
	"reflect"
	"testing"
)

const (
	testFloatVal    = 3.14
	testFloatString = "3.14"
	testIntVal      = 69
	testIntString   = "69"
	testBoolVal     = true
	testBoolString  = "true"
	testStringVal   = "Yasha and Masha are my lovest cats"
)

type testInterface interface {
	Me()
}

type testFieldStruct struct {
	Val string `json:"val"`
}

func (fs *testFieldStruct) Me() {}

func TestReaderFuncs(t *testing.T) {
	gentestfunc := func(data any, equaldata string) func(t *testing.T) {
		return func(t *testing.T) {
			val := reflect.ValueOf(data)
			byteeq := []byte(equaldata)
			torfunc, err := getToReaderFunc(val.Type())
			if err != nil {
				t.Fatal(err)
			}

			r, err := torfunc(val)
			if err != nil {
				t.Fatal(err)
			}

			b := make([]byte, len(byteeq))
			r.Read(b)
			if !bytes.Equal(b, byteeq) {
				t.Fatal("value from read and equaldata do not match")
			}
		}
	}

	t.Run("test_float", gentestfunc(testFloatVal, testFloatString))
	t.Run("test_int", gentestfunc(testIntVal, testIntString))
	t.Run("test_bool", gentestfunc(testBoolVal, testBoolString))
	t.Run("test_string", gentestfunc(testStringVal, testStringVal))

	eqtstruct, err := json.Marshal(&testFieldStruct{testStringVal})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("test_struct", gentestfunc(testFieldStruct{testStringVal}, string(eqtstruct)))
	structarr := []testFieldStruct{{testStringVal}, {testStringVal}}
	eqtstructarr, err := json.Marshal(&structarr)
	if err != nil {
		log.Fatal(err)
	}
	t.Run("test_struct_arr", gentestfunc(structarr, string(eqtstructarr)))

	tmap := map[string]any{"t1": 3.14, "t2": 69, "t3": true, "t4": "i love coffe"}
	eqtmap, err := json.Marshal(tmap)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("test_map", gentestfunc(tmap, string(eqtmap)))

	t.Run("test_unsupported", func(t *testing.T) {
		val := reflect.TypeOf([]int{96})
		_, err := getToReaderFunc(val)
		if err == nil {
			t.Fatal("unsupported not detected")
		}
	})
}

const (
	maxBytes = 4096
)

func TestEncodings(t *testing.T) {
	t.Run("test_encodeSingle", func(t *testing.T) {
		b := &bytes.Buffer{}
		testdata := reflect.ValueOf(testStringVal)
		mw := multipart.NewWriter(b)

		err := NewEncoder(mw).encodeField(testdata, "test_field")
		mw.Close()
		if err != nil {
			log.Fatal(err)
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			log.Fatal(err)
		}

		if form.Value["test_field"][0] != testStringVal {
			t.Fatal("test data val and form data do not match")
		}
	})
	t.Run("test_encodeFile", func(t *testing.T) {
		b := &bytes.Buffer{}

		f, err := os.Open("./test/file1")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		testdata := reflect.ValueOf(f)
		mw := multipart.NewWriter(b)

		err = NewEncoder(mw).encodeField(testdata, "test_file")
		mw.Close()
		if err != nil {
			t.Fatal(err)
		}

		_, err = f.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal(err)
		}

		stat, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			t.Fatal(err)
		}

		if stat.Size() != form.File["test_file"][0].Size {
			t.Fatal("test file val and form file do not match")
		}
	})

	t.Run("test_encodeArray", func(t *testing.T) {
		b := &bytes.Buffer{}
		testarray := []string{testStringVal, "2", "3", "4", "5", "6"}
		testdata := reflect.ValueOf(testarray)
		mw := multipart.NewWriter(b)

		err := NewEncoder(mw).encodeField(testdata, "test_arr")
		mw.Close()
		if err != nil {
			t.Fatal("err")
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(testarray, form.Value["test_arr[]"]) {
			t.Fatal("test data val and form data do not match")
		}
	})

	t.Run("test_encodeStructArray", func(t *testing.T) {
		b := &bytes.Buffer{}
		testarray := &[]*testFieldStruct{{testStringVal}, {testStringVal}}
		testdata := reflect.ValueOf(testarray)

		mw := multipart.NewWriter(b)

		err := NewEncoder(mw).encodeField(testdata, "test_field")
		mw.Close()
		if err != nil {
			log.Fatal(err)
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			t.Fatal(err)
		}

		eqdatabyte, err := json.Marshal(&testarray)
		if err != nil {
			log.Fatal(err)
		}

		if form.Value["test_field"][0] != string(eqdatabyte) {
			t.Fatal("test data val and form data do not match")
		}

	})
}

type myInt int

type testStruct struct {
	EmptyInter        testInterface      `multipart:"empty_interface"`
	Inter             testInterface      `multipart:"interface"`
	FieldStruct       testFieldStruct    `multipart:"field_struct"`
	PtrFieldStruct    *testFieldStruct   `multipart:"ptr_field_struct"`
	ArrFieldStruct    []testFieldStruct  `multipart:"arr_field_struct"`
	ArrPtrFieldStruct []*testFieldStruct `multipart:"arr_ptr_field_struct"`
	EmptyArr          []struct{}         `multipart:"empty_arr"`
	TMap              map[string]any     `multipart:"map"`
	TMapPtr           *map[string]any    `multipart:"map_ptr"`
	File              *os.File           `multipart:"file"`
	Str               string             `multipart:"str"`
	PtrStr            *string            `multipart:"ptr_str"`
	Fltnum            float64            `multipart:"float"`
	PtrFltNum         *float64           `multipart:"ptr_float"`
	NumInt            int                `multipart:"int"`
	PtrNumInt         *int               `multipart:"ptr_int"`
	Boolean           bool               `multipart:"bool"`
	PtrBoolean        *bool              `multipart:"ptr_bool"`
	MyInt             myInt              `multipart:"my_int"`
	NilPtr            *struct{}          `mulripart:"nil_ptr"`
	IgnoreMe          int                `multipart:"-"`
	IgnoreMeToo       int
	iPrivate          int `multipart:"private"`
}

func TestEncode(t *testing.T) {

	ptrStr := testStringVal
	ptrFlt := testFloatVal
	ptrInt := testIntVal
	ptrBool := testBoolVal
	testMap := map[string]any{"t1": "t1", "t2": "t2"}

	t.Run("test_encodeStruct", func(t *testing.T) {
		f, err := os.Open("./test/file1")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		tstruct := &testStruct{
			Inter:             &testFieldStruct{testStringVal},
			FieldStruct:       testFieldStruct{testStringVal},
			PtrFieldStruct:    &testFieldStruct{testStringVal},
			ArrFieldStruct:    []testFieldStruct{{testStringVal}, {testStringVal}},
			ArrPtrFieldStruct: []*testFieldStruct{{testStringVal}, {testStringVal}},
			TMap:              testMap,
			TMapPtr:           &testMap,
			File:              f,
			Str:               testStringVal,
			PtrStr:            &ptrStr,
			Fltnum:            testFloatVal,
			PtrFltNum:         &ptrFlt,
			NumInt:            testIntVal,
			PtrNumInt:         &ptrInt,
			Boolean:           testBoolVal,
			PtrBoolean:        &ptrBool,
			iPrivate:          0,
		}

		b := &bytes.Buffer{}
		mw := multipart.NewWriter(b)
		err = NewEncoder(mw).Encode(tstruct)
		mw.Close()
		if err != nil {
			log.Fatal(err)
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			log.Fatal(err)
		}

		if len(form.Value) != 16 && len(form.File) != 1 {
			log.Fatal("wrong number of fields written")
		}
	})
	t.Run("test_encodeMap", func(t *testing.T) {
		f, err := os.Open("./test/file1")
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		tmap := &map[string]any{
			"FieldStruct":    testFieldStruct{testStringVal},
			"PtrFieldStruct": &testFieldStruct{testStringVal},
			"PtrStr":         &ptrStr,
			"PtrFlt":         &ptrFlt,
			"PtrInt":         &ptrInt,
			"PtrBool":        &ptrBool,
			"Map":            testMap,
			"MapPtr":         &testMap,
			"file":           f,
			"nil":            nil,
		}

		b := &bytes.Buffer{}
		mw := multipart.NewWriter(b)
		err = NewEncoder(mw).Encode(tmap)
		mw.Close()

		if err != nil {
			log.Fatal(err)
		}

		form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
		if err != nil {
			log.Fatal(err)
		}

		if len(form.Value) != 8 && len(form.File) != 1 {
			log.Fatal("wrong number of fields written")
		}
	})
}

func TestEncodeField(t *testing.T) {
	f, err := os.Open("./test/file1")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	b := &bytes.Buffer{}
	mw := multipart.NewWriter(b)
	err = NewEncoder(mw).EncodeField(f, "test_file")
	mw.Close()
	if err != nil {
		log.Fatal(err)
	}

	form, err := multipart.NewReader(b, mw.Boundary()).ReadForm(maxBytes)
	if err != nil {
		log.Fatal(err)
	}

	stat, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}

	if stat.Size() != form.File["test_file"][0].Size {
		t.Fatal("test file val and form file do not match")
	}
}
