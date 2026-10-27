// Package strictjson rejects duplicate keys, unknown and case-folded properties,
// null fields, excessive nesting and trailing JSON values.
package strictjson

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "reflect"
 "strings"
 "unicode/utf8"
)
var ErrInvalid = errors.New("invalid strict JSON")
func Unique(b []byte) error {
 d:=json.NewDecoder(bytes.NewReader(b));d.UseNumber()
 var value func(int)error
 value=func(depth int)error {
  if depth>32{return ErrInvalid};t,e:=d.Token();if e!=nil{return ErrInvalid};v,ok:=t.(json.Delim);if !ok{return nil}
  switch v {
  case '{': seen:=map[string]bool{};for d.More(){t,e:=d.Token();if e!=nil{return ErrInvalid};k,ok:=t.(string);if !ok||seen[k]{return ErrInvalid};seen[k]=true;if e=value(depth+1);e!=nil{return e}};t,e=d.Token();if e!=nil||t!=json.Delim('}'){return ErrInvalid}
  case '[': for d.More(){if e=value(depth+1);e!=nil{return e}};t,e=d.Token();if e!=nil||t!=json.Delim(']'){return ErrInvalid}
  default:return ErrInvalid
  };return nil
 }
 if e:=value(0);e!=nil{return e};if _,e:=d.Token();e!=io.EOF{return ErrInvalid};return nil
}
func Decode(b []byte,max int,dst any) error {
 if len(b)>max||!utf8.Valid(b)||Unique(b)!=nil{return ErrInvalid}
 t:=reflect.TypeOf(dst);if t==nil||t.Kind()!=reflect.Pointer{return ErrInvalid}
 if shape(b,t.Elem())!=nil{return ErrInvalid}
 d:=json.NewDecoder(bytes.NewReader(b));d.DisallowUnknownFields();if d.Decode(dst)!=nil||d.Decode(new(any))!=io.EOF{return ErrInvalid};return nil
}
func shape(b []byte,t reflect.Type) error {
 if bytes.Equal(bytes.TrimSpace(b),[]byte("null")){return ErrInvalid}
 for t.Kind()==reflect.Pointer{t=t.Elem()}
 if t==reflect.TypeOf(json.RawMessage{})||t.Kind()==reflect.Interface{return nil}
 switch t.Kind(){
 case reflect.Struct:
  var m map[string]json.RawMessage;if json.Unmarshal(b,&m)!=nil||m==nil{return ErrInvalid};allowed:=map[string]bool{}
  for i:=0;i<t.NumField();i++{f:=t.Field(i);if f.PkgPath!=""{continue};name:=strings.Split(f.Tag.Get("json"),",")[0];if name=="-"{continue};if name==""{name=f.Name};allowed[name]=true;v,ok:=m[name];if !ok{if f.Tag.Get("cm")=="required"{return ErrInvalid};continue};if shape(v,f.Type)!=nil{return ErrInvalid}}
  for k:=range m{if !allowed[k]{return ErrInvalid}}
 case reflect.Slice,reflect.Array:
  if t.Elem().Kind()==reflect.Uint8{return nil};var a []json.RawMessage;if json.Unmarshal(b,&a)!=nil{return ErrInvalid};for _,v:=range a{if shape(v,t.Elem())!=nil{return ErrInvalid}}
 case reflect.Map:
  var m map[string]json.RawMessage;if json.Unmarshal(b,&m)!=nil{return ErrInvalid};for _,v:=range m{if shape(v,t.Elem())!=nil{return ErrInvalid}}
 };return nil
}
