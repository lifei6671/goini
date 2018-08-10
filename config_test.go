package goini

import (
	"testing"
	"fmt"
	"path/filepath"
)

func TestParseValueEnv1(t *testing.T) {
	k,v := ParseValueEnv("${GOPATH}")

	if v == "" {
		t.Fail()
	}else{
		fmt.Println(k,v)
		t.Log(k,v)
	}
}

func TestParseValueEnv2(t *testing.T) {
	k,v := ParseValueEnv("${GOSRC||/etc/go/src}")

	if v == "" {
		t.Fail()
	}else{
		fmt.Println(k,v)
		t.Log(k,v)
	}
}

func TestParseValueEnv3(t *testing.T) {
	k,v := ParseValueEnv("${GOSRC}")

	if v != "" {
		t.Fail()
	}else{
		fmt.Println(k,v)
		t.Log(k,v)
	}
}
func TestLoadFromFile(t *testing.T) {
	path,_ := filepath.Abs("./testdata/data1.conf")

	ini,err := LoadFromFile(path)

	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(ini)

}