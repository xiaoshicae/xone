package xutil

import (
	"encoding/json"
)

// ToJsonString 转换为json字符串
func ToJsonString(v interface{}) string {
	vv, _ := json.Marshal(v)
	return string(vv)
}

// ToJsonStringIndent 转换为json字符串，带\t格式化
func ToJsonStringIndent(v interface{}) string {
	vv, e := json.MarshalIndent(v, "", "\t")
	if e != nil {
		ErrorIfEnableDebug("ToJsonStringIndent failed, err=[%v]", e)
		return ""
	}
	return string(vv)
}
