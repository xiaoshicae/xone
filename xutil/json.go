package xutil

import (
	"encoding/json"
)

// ToJsonString 转换为json字符串
func ToJsonString(v interface{}) string {
	vv, err := json.Marshal(v)
	if err != nil {
		ErrorIfEnableDebug("ToJsonString failed, err=[%v]", err)
		return ""
	}
	return string(vv)
}

// ToJsonStringIndent 转换为json字符串，带\t格式化
func ToJsonStringIndent(v interface{}) string {
	vv, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		ErrorIfEnableDebug("ToJsonStringIndent failed, err=[%v]", err)
		return ""
	}
	return string(vv)
}
