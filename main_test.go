package main

import (
	"bytes"
	"context"
	"testing"
)

func TestCLIHelpAndInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"search", "-h"}, {"download", "-h"}} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, &out, &errOut); err != nil || out.Len()+errOut.Len() == 0 {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"missing"}, {"search"}, {"search", "--offset", "-1", "x"}, {"search", "--timeout", "0", "x"}, {"download", "abc"}, {"download", "--interval", "-1s", "123"}, {"download", "123", "-o", "out.txt"}} {
		var out, errOut bytes.Buffer
		if err := run(context.Background(), args, &out, &errOut); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
