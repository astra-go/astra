package graphql

import (
	"fmt"
	"testing"
)

type Doc struct {
	Value int `astra:"desc:A numeric value"`
}

func TestTagDebug(t *testing.T) {
	obj := MapStruct(Doc{})
	f := obj.Fields()["Value"]
	fmt.Printf("Field: %+v\n", f)
}
