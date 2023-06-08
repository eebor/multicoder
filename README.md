# Multipart Encoder for Golang

The library lets you convert different types to mutlipart/form-data
## Install

```bash
  go get -u github.com/eebor/multicoder
```
    
## Usage/Examples

Encode struct: 
```go
package main

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"multicoder"
	"os"
)

type MyFavStruct struct {
	Num  int      `multipart:"num"`
	Flt  float32  `multipart:"float"`
	Str  string   `multipart:"string"`
	File *os.File `multipart:"file"`
}

func main() {
	f, err := os.Open("favfile")
	if err != nil {
		log.Fatal(err)
	}

	data := &MyFavStruct{23, 3.14, "This is my favorite data struct!", f}

	b := &bytes.Buffer{}
	mw := multipart.NewWriter(b)

	if err := multicoder.NewEncoder(mw).Encode(data); err != nil {
		log.Fatal(err)
	}
	mw.Close()

	fmt.Println(b.String())
}

```
Output:

```
--27ed01a388973f953b0de8fed37ad2ee2b3b88d12113bfb4d085a0031c25
Content-Disposition: form-data; name="num"

23
--27ed01a388973f953b0de8fed37ad2ee2b3b88d12113bfb4d085a0031c25
Content-Disposition: form-data; name="float"

3.140000
--27ed01a388973f953b0de8fed37ad2ee2b3b88d12113bfb4d085a0031c25
Content-Disposition: form-data; name="string"

This is my favorite data struct!
--27ed01a388973f953b0de8fed37ad2ee2b3b88d12113bfb4d085a0031c25
Content-Disposition: form-data; name="file"; filename="favfile"
Content-Type: application/octet-stream

my fav file content
--27ed01a388973f953b0de8fed37ad2ee2b3b88d12113bfb4d085a0031c25--
```

## Documentation

Supported types

| Type  | Content |
| ------------- | ------------- |
| int, uint, string, float, bool | form-data (string)  |
| map, struct, map[], struct[]  | form-data (json)  |
| *File  | application/octet-stream (string)  |
| io.Reader | Not yet supported (TODO)  |



### Tags

Using tags

```go
type MyFavStruct struct {
	Num         int      `multipart:"mynum"`
	Flt         float32  `multipart:"myfloat"`
	Str         string   `multipart:"mystr"`
	NonParseStr string   `multipart:"-"`
	File        *os.File `multipart:"myfile"`
}
```

The tag value is used for the field name

"-" is ignored when parsing

### Encode 

Used to convert struct and maps to mutlipart/form-data 

In struct, all fields with a multipart tag are encoded

In map, all keys are encoded

Declaration:
```go
func (e *Encoder) Encode(v any) error
```

Example:

```go
e := NewEncoder(*multipart.Writer)
err := e.Encode(v)
```

Returns an error in case of failure

Only accepts map and struct types
### EncodeField

Used to convert any type to multipart/form-data field

Takes the name of the field as the second argument 

Declaration:
```go
func (e *Encoder) EncodeField(v any, fieldname string) error
```

Example:

```go
e := NewEncoder(*multipart.Writer)
err := e.EncodeField(v, "field_name")
```

Returns an error in case of failure

Accepts any type
## TODO

- Add support for io.Reader
- Add support for sending []byte in one field 

