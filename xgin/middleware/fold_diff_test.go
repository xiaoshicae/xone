package middleware

import (
	"bytes"
	"math/rand"
	"testing"
)

// oldBodyMayContainSensitiveField 改动前的实现，用于差分比对
func oldBodyMayContainSensitiveField(body []byte, fieldBytes [][]byte) bool {
	lowerBody := bytes.ToLower(body)
	for _, fb := range fieldBytes {
		if bytes.Contains(lowerBody, fb) {
			return true
		}
	}
	return false
}

// TestBodyMayContainSensitiveField_SameAsOld 新旧实现必须逐例一致
//
// 判错的后果不对称：漏判意味着密码明文进日志，所以不能只测几个正例
func TestBodyMayContainSensitiveField_SameAsOld(t *testing.T) {
	fb := getSensitiveFieldBytes()

	cases := [][]byte{
		nil, {}, []byte("{}"),
		[]byte(`{"password":"x"}`),
		[]byte(`{"PASSWORD":"x"}`),
		[]byte(`{"PassWord":"x"}`),
		[]byte(`{"pass":"x"}`),
		[]byte(`{"p":"password"}`), // 值里出现也算命中，与旧实现一致
		[]byte(`{"a":"TOKEN"}`),
		[]byte(`{"api_key":1}`),
		[]byte(`{"API_KEY":1}`),
		[]byte(`{"apikey":1}`),
		[]byte(`{"Authorization":"b"}`),
		[]byte(`{"access_token":"b"}`),
		[]byte(`{"refresh_token":"b"}`),
		[]byte(`{"secret":"b"}`),
		[]byte(`{"name":"alice","age":3}`),
		[]byte("passwor"),             // 差一个字符，不应命中
		[]byte("token"),               // 恰好等于字段名
		[]byte("TOKE"),                // 前缀，不应命中
		[]byte("xtokenx"),             // 内嵌
		[]byte("\x00\xff\xfe binary"), // 非 ASCII 字节
		[]byte("密码 password 中文"),      // 多字节字符旁边
		[]byte("ToKeN"),
	}
	// 再加一批随机串，覆盖边界拼接
	r := rand.New(rand.NewSource(1))
	alphabet := []byte("abcdeFGHIJ_{}\":,0123\xff")
	for range 2000 {
		n := r.Intn(40)
		s := make([]byte, n)
		for i := range s {
			s[i] = alphabet[r.Intn(len(alphabet))]
		}
		// 一半样本里随机插入一个敏感字段名的随机大小写形态
		if r.Intn(2) == 0 && len(fb) > 0 {
			f := fb[r.Intn(len(fb))]
			mixed := make([]byte, len(f))
			for i, c := range f {
				if r.Intn(2) == 0 {
					mixed[i] = upperASCII(c)
				} else {
					mixed[i] = c
				}
			}
			pos := r.Intn(len(s) + 1)
			s = append(s[:pos:pos], append(mixed, s[pos:]...)...)
		}
		cases = append(cases, s)
	}

	for _, body := range cases {
		want := oldBodyMayContainSensitiveField(body, fb)
		got := bodyMayContainSensitiveField(body, fb)
		if want != got {
			t.Fatalf("行为不一致 body=%q 旧=%v 新=%v", body, want, got)
		}
	}
}
