package session

import (
	"encoding/json"
	"strings"
)
// Tool opt-in never opts into binary attachments. Known embedded attachment forms are removed before persistence.
func textToolBody(body string)(string,bool){body=strings.ToValidUTF8(body,"�");for _,prefix:=range []string{"data:image/","data:audio/","data:video/","data:application/pdf"}{if strings.Contains(strings.ToLower(body),prefix){return "[attachment excluded]",true}};var value any;if json.Unmarshal([]byte(body),&value)!=nil{return body,false};filtered,omitted:=filterTool(value,0);if !omitted{return body,false};b,e:=json.Marshal(filtered);if e!=nil{return "[attachment excluded]",true};return string(b),true}
func filterTool(v any,depth int)(any,bool){if depth>32{return "[structured body excluded]",true};switch x:=v.(type){case map[string]any:if kind,ok:=x["type"].(string);ok{switch kind{case "image","image_url","input_image","document","audio","input_audio","base64":return "[attachment excluded]",true}};out:=map[string]any{};removed:=false;for key,value:=range x{switch strings.ToLower(key){case "attachments","image","images","image_url","audio","video":removed=true;continue};clean,omit:=filterTool(value,depth+1);out[key]=clean;removed=removed||omit};return out,removed
	case []any:out:=make([]any,0,len(x));removed:=false;for _,value:=range x{clean,omit:=filterTool(value,depth+1);out=append(out,clean);removed=removed||omit};return out,removed
	default:return v,false}}
